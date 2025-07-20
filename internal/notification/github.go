package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	appconfig "kindle_bot/internal/config"
	"kindle_bot/pkg/models"
)

func UpdateGist(gistID, filename, markdown string) error {
	payload := models.GistPayload{
		Files: models.GistFiles{
			filename: {
				Content: markdown,
			},
		},
	}

	url := fmt.Sprintf("https://api.github.com/gists/%s", gistID)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "token "+appconfig.EnvConfig.GitHubToken)
	req.Header.Set("Content-Type", "application/json")

	var client http.Client
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
