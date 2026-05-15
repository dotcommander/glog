package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
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

func InboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Manage agent inbox notes and tasks",
	}
	cmd.AddCommand(inboxAddCmd(), inboxListCmd(), inboxDoneCmd())
	return cmd
}

func inboxAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add an inbox item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := resolveServer(cmd, "")
			if err != nil {
				return err
			}
			body, _ := cmd.Flags().GetString("body")
			status, _ := cmd.Flags().GetString("status")
			kind, _ := cmd.Flags().GetString("kind")
			priority, _ := cmd.Flags().GetString("priority")
			source, _ := cmd.Flags().GetString("source")
			repo, _ := cmd.Flags().GetString("repo")
			sessionID, _ := cmd.Flags().GetString("session")
			createdBy, _ := cmd.Flags().GetString("created-by")
			tags, _ := cmd.Flags().GetStringSlice("tag")

			item, err := createInboxItem(server, map[string]any{
				"title":      args[0],
				"body":       body,
				"status":     status,
				"kind":       kind,
				"priority":   priority,
				"source":     source,
				"repo":       repo,
				"session_id": sessionID,
				"created_by": createdBy,
				"tags":       tags,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Created inbox item #%d: %s\n", item.ID, item.Title)
			return nil
		},
	}
	cmd.Flags().String("server", "", "GLog server URL")
	cmd.Flags().String("body", "", "Inbox item body")
	cmd.Flags().String("status", "open", "Status: open, in_progress, blocked, done, dismissed")
	cmd.Flags().String("kind", "task", "Kind: note, task, question, bug, handoff")
	cmd.Flags().String("priority", "normal", "Priority: low, normal, high")
	cmd.Flags().String("source", "agent", "Source: agent, human, system")
	cmd.Flags().String("repo", "", "Repository or workspace")
	cmd.Flags().String("session", "", "Agent session ID")
	cmd.Flags().String("created-by", "", "Creator name")
	cmd.Flags().StringSlice("tag", nil, "Tags for the inbox item")
	return cmd
}

func inboxListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List inbox items",
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := resolveServer(cmd, "")
			if err != nil {
				return err
			}
			status, _ := cmd.Flags().GetString("status")
			kind, _ := cmd.Flags().GetString("kind")
			repo, _ := cmd.Flags().GetString("repo")
			sessionID, _ := cmd.Flags().GetString("session")
			items, err := listInboxItems(server, status, kind, repo, sessionID)
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
		},
	}
	cmd.Flags().String("server", "", "GLog server URL")
	cmd.Flags().String("status", "open", "Filter by status")
	cmd.Flags().String("kind", "", "Filter by kind")
	cmd.Flags().String("repo", "", "Filter by repository")
	cmd.Flags().String("session", "", "Filter by session ID")
	return cmd
}

func inboxDoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "done <id>",
		Short: "Mark an inbox item done",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			server, err := resolveServer(cmd, "")
			if err != nil {
				return err
			}
			item, err := completeInboxItem(server, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Completed inbox item #%d: %s\n", item.ID, item.Title)
			return nil
		},
	}
	cmd.Flags().String("server", "", "GLog server URL")
	return cmd
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create failed (status %d): %s", resp.StatusCode, string(body))
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list failed (status %d): %s", resp.StatusCode, string(body))
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("complete failed (status %d): %s", resp.StatusCode, string(body))
	}
	var item InboxItemResponse
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &item, nil
}
