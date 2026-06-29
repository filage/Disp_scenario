# Demo delivery dashboard

This directory contains the static Admin Portal used to record delivery workflow videos for DispScenario analysis.

## Run locally

From the `analyst-app-v2` directory:

```powershell
npx serve -s demo-dashboard -l 5173
```

Then open <http://localhost:5173/deliveries>.

The dashboard stores demo state in the browser's local storage. To reset it, open:

```text
http://localhost:5173/deliveries?reset-demo=1
```

## Contents

The HTML, CSS, and JavaScript files are a deployable static build. No backend or S3 connection is required for the demo dashboard itself.
