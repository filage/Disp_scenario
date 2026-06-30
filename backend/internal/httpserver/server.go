package httpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/example/dispscenario-analyst-v2/internal/analysis"
	"github.com/example/dispscenario-analyst-v2/internal/artifacts"
	authn "github.com/example/dispscenario-analyst-v2/internal/auth"
	"github.com/example/dispscenario-analyst-v2/internal/credentials"
	"github.com/example/dispscenario-analyst-v2/internal/observability"
	"github.com/example/dispscenario-analyst-v2/internal/platform"
	"github.com/example/dispscenario-analyst-v2/internal/recording"
	"github.com/example/dispscenario-analyst-v2/internal/storage"
)

const healthDependencyTimeout = 2 * time.Second

type Server struct {
	router       chi.Router
	pool         *pgxpool.Pool
	recordings   *recording.Repository
	analysis     *analysis.Service
	credentials  *credentials.Store
	artifacts    *artifacts.Service
	auth         *authn.Middleware
	storage      *storage.Storage
	redis        *redis.Client
	sharedSecret string
	logger       *slog.Logger
	webOrigin    string
}

func New(
	pool *pgxpool.Pool,
	recordings *recording.Repository,
	analysisService *analysis.Service,
	credentialStore *credentials.Store,
	artifactService *artifacts.Service,
	authMiddleware *authn.Middleware,
	objectStorage *storage.Storage,
	redisClient *redis.Client,
	sharedSecret string,
	logger *slog.Logger,
	webOrigin string,
) *Server {
	server := &Server{
		pool: pool, recordings: recordings, analysis: analysisService,
		credentials: credentialStore,
		artifacts:   artifactService, auth: authMiddleware,
		storage: objectStorage, redis: redisClient, sharedSecret: sharedSecret,
		logger: logger, webOrigin: webOrigin,
	}
	server.router = server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) routes() chi.Router {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(30 * time.Second))
	router.Use(s.cors)
	router.Use(observability.HTTPMiddleware(s.logger))

	router.Get("/health", s.health)
	router.Handle("/metrics", promhttp.Handler())
	router.Route("/v1", func(router chi.Router) {
		router.Use(s.requireSharedSecret)
		router.Use(middleware.Throttle(100))
		router.Use(newIPRateLimiter(300, time.Minute).Middleware)
		router.Use(s.auth.Authenticate)
		router.Use(s.auth.Authorize("viewer"))
		router.Get("/recordings", s.listRecordings)
		router.Post("/recordings/uploads", s.auth.Require("analyst", s.createUpload))
		router.Post("/recordings/{recordingID}/uploads/complete", s.auth.Require("analyst", s.completeUpload))
		router.Get("/recordings/{recordingID}/playback", s.playback)
		router.Get("/recordings/{recordingID}/evidence/{timestampMS}", s.evidence)
		router.Delete("/recordings/{recordingID}", s.auth.Require("admin", s.deleteRecording))
		router.Post("/recordings/{recordingID}/analysis-runs", s.auth.Require("analyst", s.createAnalysisRun))
		router.Post("/recordings/{recordingID}/rebuild", s.auth.Require("analyst", s.rebuild))
		router.Post("/recordings/{recordingID}/renormalize", s.auth.Require("analyst", s.renormalize))
		router.Post("/recordings/{recordingID}/boundary-review", s.auth.Require("analyst", s.boundaryReview))
		router.Get("/recordings/{recordingID}/events", s.events)
		router.Get("/recordings/{recordingID}/raw-events", s.rawEvents)
		router.Get("/recordings/{recordingID}/quality-issues", s.qualityIssues)
		router.Get("/recordings/{recordingID}/analysis", s.analysisBundle)
		router.Get("/recordings/{recordingID}/scenario-map", s.scenarioMap)
		router.Patch("/recordings/{recordingID}/events/{eventID}", s.auth.Require("analyst", s.patchEvent))
		router.Patch("/recordings/{recordingID}/quality-issues/{issueID}", s.auth.Require("analyst", s.patchQualityIssue))
		router.Post("/recordings/{recordingID}/qa/complete", s.auth.Require("analyst", s.completeQA))
		router.Get("/recordings/{recordingID}/exports/{kind}.{format}", s.export)
		router.Get("/analysis-runs", s.listAnalysisRuns)
		router.Get("/analysis-runs/{runID}", s.getAnalysisRun)
		router.Post("/analysis-runs/{runID}/cancel", s.auth.Require("analyst", s.cancelAnalysisRun))
		router.Post("/analysis-runs/{runID}/retry", s.auth.Require("analyst", s.retryAnalysisRun))
		router.Get("/scenarios", s.scenarios)
		router.Get("/project/analysis", s.projectAnalysis)
		router.Get("/reports/{reportID}", s.report)
		router.Post("/ground-truth/import", s.auth.Require("analyst", s.importGroundTruth))
		router.Get("/settings", s.settings)
		router.Get("/settings/gemini-credential", s.geminiCredentialStatus)
		router.Put("/settings/gemini-credential", s.auth.Require("analyst", s.saveGeminiCredential))
		router.Delete("/settings/gemini-credential", s.auth.Require("analyst", s.deleteGeminiCredential))
		router.Patch("/settings/known-scenarios/{code}", s.auth.Require("admin", s.patchKnownScenario))
		router.Patch("/settings/boundary-rules/{ruleID}", s.auth.Require("admin", s.patchBoundaryRule))
	})
	return router
}

func (s *Server) requireSharedSecret(next http.Handler) http.Handler {
	if s.sharedSecret == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get("X-API-Shared-Secret")
		if len(provided) != len(s.sharedSecret) ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(s.sharedSecret)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid API credentials")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	dependencies := map[string]string{"postgres": "ok", "s3": "ok"}
	status := "ok"

	postgresCtx, postgresCancel := context.WithTimeout(r.Context(), healthDependencyTimeout)
	if err := s.pool.Ping(postgresCtx); err != nil {
		dependencies["postgres"] = "error"
		status = "degraded"
	}
	postgresCancel()

	s3Ctx, s3Cancel := context.WithTimeout(r.Context(), healthDependencyTimeout)
	if err := s.storage.EnsureBucket(s3Ctx); err != nil {
		dependencies["s3"] = "error"
		status = "degraded"
	}
	s3Cancel()

	if s.redis == nil {
		dependencies["job_queue"] = "postgres"
	} else {
		dependencies["redis"] = "ok"
		redisStarted := time.Now()
		redisCtx, redisCancel := context.WithTimeout(r.Context(), healthDependencyTimeout)
		redisErr := s.redis.Ping(redisCtx).Err()
		redisCancel()
		observability.ObserveDependency("redis", "ping", redisStarted, redisErr)
		if redisErr != nil {
			dependencies["redis"] = "error"
			status = "degraded"
		}
	}
	httpStatus := http.StatusOK
	if status != "ok" {
		httpStatus = http.StatusServiceUnavailable
	}
	writeJSON(w, httpStatus, map[string]any{
		"status": status, "version": "0.1.0", "dependencies": dependencies,
	})
}

func (s *Server) listRecordings(w http.ResponseWriter, r *http.Request) {
	items, err := s.recordings.List(r.Context(), platform.LocalOrganizationID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type createUploadRequest struct {
	OriginalName string `json:"originalName"`
	MimeType     string `json:"mimeType"`
	SizeBytes    int64  `json:"sizeBytes"`
}

func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) {
	var input createUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if input.OriginalName == "" || input.SizeBytes <= 0 || input.SizeBytes > 2*1024*1024*1024 {
		writeError(w, http.StatusBadRequest, "invalid recording metadata")
		return
	}
	if input.MimeType != "video/webm" && input.MimeType != "video/mp4" {
		writeError(w, http.StatusBadRequest, "only video/webm and video/mp4 are supported")
		return
	}

	id := uuid.New()
	extension := ".webm"
	if input.MimeType == "video/mp4" {
		extension = ".mp4"
	}
	objectKey := "recordings/" + id.String() + "/source" + extension
	item, err := s.recordings.Create(
		r.Context(), platform.LocalOrganizationID, input.OriginalName,
		input.MimeType, input.SizeBytes, objectKey, authn.Actor(r.Context()),
	)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	expiresAt := time.Now().Add(15 * time.Minute)
	uploadURL, err := s.storage.PresignPut(r.Context(), objectKey, 15*time.Minute)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"recording": item, "uploadUrl": uploadURL.String(), "expiresAt": expiresAt,
	})
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "recordingID"))
	if !ok {
		return
	}
	item, err := s.recordings.Get(r.Context(), platform.LocalOrganizationID, id)
	if err != nil {
		s.recordingError(w, err)
		return
	}
	info, err := s.storage.Stat(r.Context(), item.ObjectKey)
	if err != nil {
		writeError(w, http.StatusConflict, "uploaded object is not available")
		return
	}
	if info.Size != item.SizeBytes {
		writeError(w, http.StatusConflict, "uploaded object size does not match")
		return
	}
	item, err = s.recordings.Complete(r.Context(), platform.LocalOrganizationID, id, authn.Actor(r.Context()))
	if err != nil {
		s.recordingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) playback(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "recordingID"))
	if !ok {
		return
	}
	item, err := s.recordings.Get(r.Context(), platform.LocalOrganizationID, id)
	if err != nil {
		s.recordingError(w, err)
		return
	}
	expiresAt := time.Now().Add(15 * time.Minute)
	signedURL, err := s.storage.PresignGet(r.Context(), item.ObjectKey, 15*time.Minute)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": signedURL.String(), "expiresAt": expiresAt})
}

func (s *Server) deleteRecording(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "recordingID"))
	if !ok {
		return
	}
	_, err := s.recordings.Delete(r.Context(), platform.LocalOrganizationID, id)
	if err != nil {
		s.recordingError(w, err)
		return
	}
	if err := s.storage.DeletePrefix(r.Context(), "recordings/"+id.String()+"/"); err != nil {
		observability.CleanupFailures.WithLabelValues("recording_objects").Inc()
		s.logger.Warn("s3 prefix cleanup failed", "recording_id", id, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createAnalysisRun(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, chi.URLParam(r, "recordingID"))
	if !ok {
		return
	}
	item, err := s.recordings.Get(r.Context(), platform.LocalOrganizationID, id)
	if err != nil {
		s.recordingError(w, err)
		return
	}
	if item.Status != "UPLOADED" && item.Status != "ANALYZED" && item.Status != "FAILED" {
		writeError(w, http.StatusConflict, "recording is not ready for analysis")
		return
	}
	if !s.requireGeminiCredential(w, r) {
		return
	}
	run, err := s.analysis.Create(r.Context(), platform.LocalOrganizationID, id, authn.Actor(r.Context()))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) requireGeminiCredential(w http.ResponseWriter, r *http.Request) bool {
	_, ok := s.resolveGeminiCredential(w, r)
	return ok
}

func (s *Server) resolveGeminiCredential(w http.ResponseWriter, r *http.Request) (string, bool) {
	apiKey, err := s.credentials.Resolve(
		r.Context(), platform.LocalOrganizationID, authn.Actor(r.Context()), "gemini",
	)
	if err != nil {
		if errors.Is(err, credentials.ErrNotFound) {
			writeError(w, http.StatusConflict, "configure your Gemini API key in settings before starting analysis")
			return "", false
		}
		s.fail(w, r, err)
		return "", false
	}
	return apiKey, true
}

type saveGeminiCredentialRequest struct {
	APIKey string `json:"apiKey"`
}

func (s *Server) geminiCredentialStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.credentials.GetStatus(
		r.Context(), platform.LocalOrganizationID, authn.Actor(r.Context()), "gemini",
	)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) saveGeminiCredential(w http.ResponseWriter, r *http.Request) {
	var input saveGeminiCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	status, err := s.credentials.Upsert(
		r.Context(), platform.LocalOrganizationID, authn.Actor(r.Context()), "gemini", input.APIKey,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid Gemini API key")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) deleteGeminiCredential(w http.ResponseWriter, r *http.Request) {
	if err := s.credentials.Delete(
		r.Context(), platform.LocalOrganizationID, authn.Actor(r.Context()), "gemini",
	); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAnalysisRuns(w http.ResponseWriter, r *http.Request) {
	var recordingID *uuid.UUID
	if value := r.URL.Query().Get("recordingId"); value != "" {
		id, err := uuid.Parse(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid recordingId")
			return
		}
		recordingID = &id
	}
	items, err := s.analysis.List(r.Context(), platform.LocalOrganizationID, recordingID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.webOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, X-Correlation-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recordingError(w http.ResponseWriter, err error) {
	if errors.Is(err, recording.ErrNotFound) {
		writeError(w, http.StatusNotFound, "recording not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.Error("request failed",
		"error", err,
		"method", r.Method,
		"path", r.URL.Path,
		"request_id", middleware.GetReqID(r.Context()),
	)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func parseID(w http.ResponseWriter, value string) (uuid.UUID, bool) {
	id, err := uuid.Parse(value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return uuid.Nil, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    strconv.Itoa(status),
			"message": strings.TrimSpace(message),
		},
	})
}
