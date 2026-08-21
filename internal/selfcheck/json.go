package selfcheck

import "encoding/json"

// jsonUnmarshal is a thin shim so scenario files that decode bodies don't each
// import encoding/json (keeps the import surface minimal in the scenario
// files).
func jsonUnmarshal(body []byte, v any) error {
	return json.Unmarshal(body, v)
}
