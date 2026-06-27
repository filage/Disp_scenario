package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/example/dispscenario-analyst-v2/internal/artifacts"
	authn "github.com/example/dispscenario-analyst-v2/internal/auth"
	"github.com/example/dispscenario-analyst-v2/internal/media"
	"github.com/example/dispscenario-analyst-v2/internal/platform"
)

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	items, err := s.artifacts.Events(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) rawEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	items, err := s.artifacts.RawEvents(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) qualityIssues(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	items, err := s.artifacts.Issues(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) analysisBundle(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	bundle, err := s.artifacts.Bundle(r.Context(), id)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) scenarioMap(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	graph, err := s.artifacts.Graph(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

func (s *Server) scenarios(w http.ResponseWriter, r *http.Request) {
	instances, err := s.artifacts.Instances(r.Context(), nil)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	templates, err := s.artifacts.Templates(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates, "instances": instances})
}

func (s *Server) projectAnalysis(w http.ResponseWriter, r *http.Request) {
	bundle, err := s.artifacts.ProjectAnalysis(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) getAnalysisRun(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "runID")
	if !ok {
		return
	}
	item, err := s.analysis.Get(r.Context(), platform.LocalOrganizationID, id)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) cancelAnalysisRun(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "runID")
	if !ok {
		return
	}
	item, err := s.analysis.Cancel(r.Context(), platform.LocalOrganizationID, id)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) retryAnalysisRun(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "runID")
	if !ok {
		return
	}
	item, err := s.analysis.Retry(r.Context(), platform.LocalOrganizationID, id, authn.Actor(r.Context()))
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "reportID")
	if !ok {
		return
	}
	item, err := s.artifacts.Report(r.Context(), id)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) rebuild(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	bundle, err := s.artifacts.Rebuild(r.Context(), id)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) renormalize(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	bundle, err := s.artifacts.Renormalize(r.Context(), id)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) boundaryReview(w http.ResponseWriter, r *http.Request) {
	id, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	bundle, err := s.artifacts.AddBoundaryReviewIssue(r.Context(), id)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) patchEvent(w http.ResponseWriter, r *http.Request) {
	recordingID, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	eventID, ok := routeID(w, r, "eventID")
	if !ok {
		return
	}
	var patch artifacts.EventPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	item, err := s.artifacts.PatchEvent(r.Context(), recordingID, eventID, patch)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	bundle, err := s.artifacts.Rebuild(r.Context(), recordingID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": item, "event": item, "bundle": bundle})
}

func (s *Server) patchQualityIssue(w http.ResponseWriter, r *http.Request) {
	recordingID, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	issueID, ok := routeID(w, r, "issueID")
	if !ok {
		return
	}
	var patch artifacts.IssuePatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	item, err := s.artifacts.PatchIssue(r.Context(), recordingID, issueID, patch)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	bundle, err := s.artifacts.Bundle(r.Context(), recordingID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": item, "issue": item, "bundle": bundle})
}

func (s *Server) completeQA(w http.ResponseWriter, r *http.Request) {
	recordingID, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	var input struct {
		IssueIDs []uuid.UUID `json:"issueIds"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if err := s.artifacts.CompleteQA(r.Context(), recordingID, input.IssueIDs); err != nil {
		s.fail(w, r, err)
		return
	}
	bundle, err := s.artifacts.Bundle(r.Context(), recordingID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bundle)
}

func (s *Server) importGroundTruth(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RecordingID *uuid.UUID        `json:"recordingId"`
		Events      []json.RawMessage `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	count, err := s.artifacts.ImportGroundTruth(r.Context(), input.RecordingID, input.Events)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"imported": count, "events": input.Events})
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	value, err := s.artifacts.Settings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) patchKnownScenario(w http.ResponseWriter, r *http.Request) {
	var patch artifacts.KnownScenarioPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	item, err := s.artifacts.PatchKnownScenario(
		r.Context(), chi.URLParam(r, "code"), patch, authn.Actor(r.Context()),
	)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) patchBoundaryRule(w http.ResponseWriter, r *http.Request) {
	var patch artifacts.BoundaryRulePatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	item, err := s.artifacts.PatchBoundaryRule(
		r.Context(), chi.URLParam(r, "ruleID"), patch, authn.Actor(r.Context()),
	)
	if err != nil {
		s.artifactError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	recordingID, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	kind, format := chi.URLParam(r, "kind"), chi.URLParam(r, "format")
	if kind != "timeline" && kind != "report" {
		writeError(w, http.StatusBadRequest, "unsupported export kind")
		return
	}
	if format != "json" && format != "csv" {
		writeError(w, http.StatusBadRequest, "unsupported export format")
		return
	}
	var payload any
	if kind == "timeline" {
		items, err := s.artifacts.Events(r.Context(), recordingID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		payload = items
	} else {
		report, err := s.artifacts.LatestReport(r.Context(), recordingID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		templates, err := s.artifacts.TemplatesForRecording(r.Context(), recordingID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		payload = map[string]any{"report": report, "scenarioGroups": templates}
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s-%s.%s"`, kind, recordingID, format,
	))
	if format == "json" {
		writeJSON(w, http.StatusOK, payload)
		return
	}
	data, _ := json.Marshal(payload)
	var rows []map[string]any
	if kind == "timeline" {
		_ = json.Unmarshal(data, &rows)
	} else {
		var envelope struct {
			ScenarioGroups []map[string]any `json:"scenarioGroups"`
		}
		_ = json.Unmarshal(data, &envelope)
		rows = envelope.ScenarioGroups
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(artifacts.CSV(rows)))
}

func (s *Server) evidence(w http.ResponseWriter, r *http.Request) {
	recordingID, ok := routeID(w, r, "recordingID")
	if !ok {
		return
	}
	timestampMS, err := strconv.Atoi(chi.URLParam(r, "timestampMS"))
	if err != nil || timestampMS < 0 {
		writeError(w, http.StatusBadRequest, "invalid evidence timestamp")
		return
	}
	recording, err := s.recordings.Get(r.Context(), platform.LocalOrganizationID, recordingID)
	if err != nil {
		s.recordingError(w, err)
		return
	}
	key := fmt.Sprintf("recordings/%s/evidence/%d.jpg", recordingID, timestampMS)
	if _, err := s.storage.Stat(r.Context(), key); err != nil {
		tempDir, tempErr := os.MkdirTemp("", "evidence-"+recordingID.String()+"-")
		if tempErr != nil {
			s.fail(w, r, tempErr)
			return
		}
		defer func() {
			if err := os.RemoveAll(tempDir); err != nil {
				s.logger.Warn("temporary evidence directory cleanup failed", "path", tempDir, "error", err)
			}
		}()
		extension := ".mp4"
		if recording.MimeType == "video/webm" {
			extension = ".webm"
		}
		videoPath := filepath.Join(tempDir, "source"+extension)
		framePath := filepath.Join(tempDir, "frame.jpg")
		if err := s.storage.Download(r.Context(), recording.ObjectKey, videoPath); err != nil {
			s.fail(w, r, err)
			return
		}
		if err := media.ExtractEvidenceFrame(r.Context(), videoPath, framePath, timestampMS); err != nil {
			s.fail(w, r, err)
			return
		}
		if _, err := s.recordings.Get(r.Context(), platform.LocalOrganizationID, recordingID); err != nil {
			s.recordingError(w, err)
			return
		}
		if err := s.storage.Upload(r.Context(), key, framePath, "image/jpeg"); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	url, err := s.storage.PresignGet(r.Context(), key, 15*time.Minute)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, url.String(), http.StatusTemporaryRedirect)
}

func routeID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	return parseID(w, chi.URLParam(r, name))
}

func (s *Server) artifactError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, artifacts.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	if strings.Contains(err.Error(), "must be") {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.fail(w, r, err)
}
