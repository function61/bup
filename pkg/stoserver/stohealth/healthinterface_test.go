package stohealth

import (
	"encoding/json"
	"testing"

	"github.com/function61/gokit/assert"
	"github.com/function61/varasto/pkg/stoserver/stoservertypes"
)

func TestBasic(t *testing.T) {
	g, _ := getTestGraph()

	jsonBytes, _ := json.MarshalIndent(g, "", "  ")

	assert.EqualString(t, string(jsonBytes), `{
  "Children": [
    {
      "Children": [
        {
          "Children": [],
          "Details": "",
          "Health": "pass",
          "Kind": null,
          "Title": "Dummy 1"
        },
        {
          "Children": [],
          "Details": "",
          "Health": "warn",
          "Kind": null,
          "Title": "Dummy 2"
        }
      ],
      "Details": "",
      "Health": "warn",
      "Kind": null,
      "Title": "SMART"
    }
  ],
  "Details": "",
  "Health": "warn",
  "Kind": null,
  "Title": "Varasto"
}`)
}

func getTestGraph() (*stoservertypes.Health, error) {
	root := NewHealthFolder(
		"Varasto",
		nil,
		NewHealthFolder("SMART",
			nil,
			NewStaticHealthNode("Dummy 1", stoservertypes.HealthStatusPass, "", nil),
			NewStaticHealthNode("Dummy 2", stoservertypes.HealthStatusWarn, "", nil)))

	return root.CheckHealth()
}
