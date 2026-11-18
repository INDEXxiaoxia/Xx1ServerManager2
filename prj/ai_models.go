package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func (a *App) ListAIModels(baseURL, apiKey string) ([]string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = aiDefaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("接口地址无效")
	}
	if apiKey == "" {
		saved, _ := a.GetAISettings()
		if strings.TrimRight(saved.BaseURL, "/") != baseURL && !(saved.BaseURL == "" && baseURL == aiDefaultBaseURL) {
			return nil, fmt.Errorf("地址已改变，请填写该地址对应的 API Key")
		}
		enc, _ := a.store.GetSetting(aiKeyAPIKey)
		if enc != "" {
			apiKey, err = a.crypto.DecryptPassword(enc)
			if err != nil {
				return nil, err
			}
		}
	}
	baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
	req, err := http.NewRequest("GET", baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("获取模型列表失败：HTTP %d", resp.StatusCode)
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&data); err != nil {
		return nil, err
	}
	result := []string{}
	seen := map[string]bool{}
	for _, m := range data.Data {
		if m.ID != "" && !seen[m.ID] {
			result = append(result, m.ID)
			seen[m.ID] = true
		}
	}
	sort.Strings(result)
	return result, nil
}
