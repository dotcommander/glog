package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type InboxItemResponse struct {
	ID        int64    `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Status    string   `json:"status"`
	Kind      string   `json:"kind"`
	Priority  string   `json:"priority"`
	Source    string   `json:"source"`
	Repo      string   `json:"repo"`
	SessionID string   `json:"session_id"`
	Tags      []string `json:"tags"`
	CreatedBy string   `json:"created_by"`
	ClaimedBy string   `json:"claimed_by"`
	UpdatedAt string   `json:"updated_at"`
}

type InboxCommand struct {
	Add  InboxAddCommand  `cmd:"" help:"Add an inbox item."`
	List InboxListCommand `cmd:"" help:"List inbox items."`
	Done InboxDoneCommand `cmd:"" help:"Mark an inbox item done."`
}

type InboxAddCommand struct {
	Title     string   `arg:"" help:"Inbox item title."`
	Server    string   `help:"GLog server URL."`
	Config    string   `default:"./glog.json" type:"path" help:"Path to the configuration file."`
	Body      string   `help:"Inbox item body."`
	Status    string   `default:"open" help:"Item status."`
	Kind      string   `default:"task" help:"Item kind."`
	Priority  string   `default:"normal" help:"Item priority."`
	Source    string   `default:"agent" help:"Item source."`
	Repo      string   `help:"Repository or workspace."`
	Session   string   `help:"Agent session ID."`
	CreatedBy string   `name:"created-by" help:"Creator name."`
	Tags      []string `name:"tag" help:"Item tag."`
}

func (c *InboxAddCommand) Run() error {
	server, err := resolveServer(c.Server, c.Config)
	if err != nil {
		return err
	}
	item, err := createInboxItem(server, map[string]any{
		"title": c.Title, "body": c.Body, "status": c.Status, "kind": c.Kind,
		"priority": c.Priority, "source": c.Source, "repo": c.Repo,
		"session_id": c.Session, "created_by": c.CreatedBy, "tags": c.Tags,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Created inbox item #%d: %s\n", item.ID, item.Title)
	return nil
}

type InboxListCommand struct {
	Server  string `help:"GLog server URL."`
	Config  string `default:"./glog.json" type:"path" help:"Path to the configuration file."`
	Status  string `default:"open" help:"Filter by status."`
	Kind    string `help:"Filter by kind."`
	Repo    string `help:"Filter by repository."`
	Session string `help:"Filter by session ID."`
}

func (c *InboxListCommand) Run() error {
	server, err := resolveServer(c.Server, c.Config)
	if err != nil {
		return err
	}
	items, err := listInboxItems(server, c.Status, c.Kind, c.Repo, c.Session)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("No inbox items")
		return nil
	}
	for _, item := range items {
		fmt.Printf("#%d [%s/%s/%s] %s\n", item.ID, item.Status, item.Kind, item.Priority, item.Title)
		meta := []string{}
		if item.Repo != "" {
			meta = append(meta, "repo="+item.Repo)
		}
		if item.SessionID != "" {
			meta = append(meta, "session="+item.SessionID)
		}
		if item.Source != "" {
			meta = append(meta, "source="+item.Source)
		}
		if len(meta) > 0 {
			fmt.Printf("  %s\n", strings.Join(meta, " "))
		}
		if item.Body != "" {
			fmt.Printf("  %s\n", item.Body)
		}
	}
	return nil
}

type InboxDoneCommand struct {
	ID     string `arg:"" help:"Inbox item ID."`
	Server string `help:"GLog server URL."`
	Config string `default:"./glog.json" type:"path" help:"Path to the configuration file."`
}

func (c *InboxDoneCommand) Run() error {
	server, err := resolveServer(c.Server, c.Config)
	if err != nil {
		return err
	}
	item, err := completeInboxItem(server, c.ID)
	if err != nil {
		return err
	}
	fmt.Printf("Completed inbox item #%d: %s\n", item.ID, item.Title)
	return nil
}

func createInboxItem(server string, body map[string]any) (*InboxItemResponse, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequest("POST", server+"/api/v1/inbox", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create failed (status %d): %s", resp.StatusCode, readResponseBodySnippet(resp.Body))
	}
	var item InboxItemResponse
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &item, nil
}

func listInboxItems(server, status, kind, repo, sessionID string) ([]InboxItemResponse, error) {
	values := url.Values{}
	if status != "" {
		values.Set("status", status)
	}
	if kind != "" {
		values.Set("kind", kind)
	}
	if repo != "" {
		values.Set("repo", repo)
	}
	if sessionID != "" {
		values.Set("session_id", sessionID)
	}
	reqURL := server + "/api/v1/inbox"
	if encoded := values.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list failed (status %d): %s", resp.StatusCode, readResponseBodySnippet(resp.Body))
	}
	var result struct {
		Items []InboxItemResponse `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result.Items, nil
}

func completeInboxItem(server, id string) (*InboxItemResponse, error) {
	req, err := http.NewRequest("POST", server+"/api/v1/inbox/"+id+"/done", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("complete failed (status %d): %s", resp.StatusCode, readResponseBodySnippet(resp.Body))
	}
	var item InboxItemResponse
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &item, nil
}
