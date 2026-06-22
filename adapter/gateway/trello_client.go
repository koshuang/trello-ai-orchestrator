package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/koshuang/trello-ai-orchestrator/config"
	"github.com/koshuang/trello-ai-orchestrator/domain"
)

type TrelloClient struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewTrelloClient(cfg *config.Config) *TrelloClient {
	return &TrelloClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type rawTrelloCard struct {
	ID       string               `json:"id"`
	Name     string               `json:"name"`
	Desc     string               `json:"desc"`
	ShortURL string               `json:"shortUrl"`
	Labels   []domain.TrelloLabel `json:"labels"`
}

type rawTrelloComment struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Date          time.Time `json:"date"`
	MemberCreator struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		FullName string `json:"fullName"`
	} `json:"memberCreator"`
	Data struct {
		Text string `json:"text"`
	} `json:"data"`
}

// FetchCardContext aggregates card info, checklists, attachments, comments, and members
func (c *TrelloClient) FetchCardContext(ctx context.Context, cardID string) (*domain.TrelloCardContext, error) {
	// 1. Fetch primary card info
	cardURL := fmt.Sprintf("https://api.trello.com/1/cards/%s?key=%s&token=%s", cardID, c.cfg.TrelloAPIKey, c.cfg.TrelloToken)
	req, err := http.NewRequestWithContext(ctx, "GET", cardURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trello card fetch returned status: %d", resp.StatusCode)
	}

	var rawCard rawTrelloCard
	if err := json.NewDecoder(resp.Body).Decode(&rawCard); err != nil {
		return nil, err
	}

	// 2. Fetch Checklists (failure does not stop the flow, fallback to empty)
	var checklists []domain.TrelloChecklist
	checklistsURL := fmt.Sprintf("https://api.trello.com/1/cards/%s/checklists?key=%s&token=%s", cardID, c.cfg.TrelloAPIKey, c.cfg.TrelloToken)
	if req2, err := http.NewRequestWithContext(ctx, "GET", checklistsURL, nil); err == nil {
		if resp2, err := c.httpClient.Do(req2); err == nil {
			defer resp2.Body.Close()
			if resp2.StatusCode == http.StatusOK {
				_ = json.NewDecoder(resp2.Body).Decode(&checklists)
			}
		}
	}

	// 3. Fetch Attachments
	var attachments []domain.TrelloAttachment
	attachmentsURL := fmt.Sprintf("https://api.trello.com/1/cards/%s/attachments?key=%s&token=%s", cardID, c.cfg.TrelloAPIKey, c.cfg.TrelloToken)
	if req3, err := http.NewRequestWithContext(ctx, "GET", attachmentsURL, nil); err == nil {
		if resp3, err := c.httpClient.Do(req3); err == nil {
			defer resp3.Body.Close()
			if resp3.StatusCode == http.StatusOK {
				_ = json.NewDecoder(resp3.Body).Decode(&attachments)
			}
		}
	}

	// 4. Fetch comments (Recent Trello card actions of type commentCard)
	var comments []domain.TrelloCommentInfo
	commentsURL := fmt.Sprintf("https://api.trello.com/1/cards/%s/actions?filter=commentCard&limit=10&key=%s&token=%s", cardID, c.cfg.TrelloAPIKey, c.cfg.TrelloToken)
	if req4, err := http.NewRequestWithContext(ctx, "GET", commentsURL, nil); err == nil {
		if resp4, err := c.httpClient.Do(req4); err == nil {
			defer resp4.Body.Close()
			if resp4.StatusCode == http.StatusOK {
				var rawComments []rawTrelloComment
				if err := json.NewDecoder(resp4.Body).Decode(&rawComments); err == nil {
					for _, rc := range rawComments {
						comments = append(comments, domain.TrelloCommentInfo{
							ID:        rc.ID,
							Text:      rc.Data.Text,
							CreatorID: rc.MemberCreator.ID,
							Username:  rc.MemberCreator.Username,
							Date:      rc.Date,
						})
					}
				}
			}
		}
	}

	// 5. Fetch Members
	var members []domain.TrelloMember
	membersURL := fmt.Sprintf("https://api.trello.com/1/cards/%s/members?key=%s&token=%s", cardID, c.cfg.TrelloAPIKey, c.cfg.TrelloToken)
	if req5, err := http.NewRequestWithContext(ctx, "GET", membersURL, nil); err == nil {
		if resp5, err := c.httpClient.Do(req5); err == nil {
			defer resp5.Body.Close()
			if resp5.StatusCode == http.StatusOK {
				_ = json.NewDecoder(resp5.Body).Decode(&members)
			}
		}
	}

	return &domain.TrelloCardContext{
		ID:          rawCard.ID,
		Title:       rawCard.Name,
		Description: rawCard.Desc,
		URL:         rawCard.ShortURL,
		Labels:      rawCard.Labels,
		Checklists:  checklists,
		Attachments: attachments,
		Comments:    comments,
		Members:     members,
	}, nil
}

// AddComment posts a new comment on a card
func (c *TrelloClient) AddComment(ctx context.Context, cardID string, text string) error {
	u := fmt.Sprintf("https://api.trello.com/1/cards/%s/actions/comments?key=%s&token=%s", cardID, c.cfg.TrelloAPIKey, c.cfg.TrelloToken)
	form := url.Values{}
	form.Add("text", text)

	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("trello add comment returned status: %d", resp.StatusCode)
	}
	return nil
}

// UpdateCardDescription updates the description markdown on a card
func (c *TrelloClient) UpdateCardDescription(ctx context.Context, cardID string, description string) error {
	u := fmt.Sprintf("https://api.trello.com/1/cards/%s?key=%s&token=%s", cardID, c.cfg.TrelloAPIKey, c.cfg.TrelloToken)
	form := url.Values{}
	form.Add("desc", description)

	req, err := http.NewRequestWithContext(ctx, "PUT", u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("trello update card description returned status: %d", resp.StatusCode)
	}
	return nil
}
