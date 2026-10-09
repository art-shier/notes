package agentassets

import (
	"embed"
	"encoding/json"
)

//go:embed files
var resources embed.FS

func Read(path string) ([]byte, string, bool) {
	contentType := "text/plain; charset=utf-8"
	switch path {
	case "SKILL.md", "references/api.md", "scripts/notes.py", "scripts/agent_auth.py", "install-client.py":
	case "shiji-notes.zip":
		contentType = "application/zip"
	case "manifest.json":
		contentType = "application/json; charset=utf-8"
	default:
		return nil, "", false
	}
	b, e := resources.ReadFile("files/" + path)
	return b, contentType, e == nil
}
func Metadata(origin string) map[string]any {
	b, _, _ := Read("manifest.json")
	v := map[string]any{}
	_ = json.Unmarshal(b, &v)
	v["skill_url"] = origin + "/agent/SKILL.md"
	v["archive_url"] = origin + "/agent/shiji-notes.zip"
	v["installer_url"] = origin + "/agent/install-client.py"
	v["api_url"] = origin + "/api/v1"
	return v
}
