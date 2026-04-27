package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	token      string
	httpClient *http.Client
	username   string
}

func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) graphQL(query string, variables map[string]interface{}, result interface{}) error {
	reqBody := graphQLRequest{
		Query:     query,
		Variables: variables,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.github.com/graphql", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var gqlResp graphQLResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}

	if result != nil {
		if err := json.Unmarshal(gqlResp.Data, result); err != nil {
			return fmt.Errorf("parsing data: %w", err)
		}
	}
	return nil
}

func (c *Client) GetUsername() (string, error) {
	if c.username != "" {
		return c.username, nil
	}
	var result struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}
	if err := c.graphQL(`query { viewer { login } }`, nil, &result); err != nil {
		return "", err
	}
	c.username = result.Viewer.Login
	return c.username, nil
}

func (c *Client) GetOrganizations() ([]string, error) {
	var result struct {
		Viewer struct {
			Organizations struct {
				Nodes []struct {
					Login string `json:"login"`
				} `json:"nodes"`
			} `json:"organizations"`
		} `json:"viewer"`
	}

	query := `query {
		viewer {
			organizations(first: 100) {
				nodes {
					login
				}
			}
		}
	}`

	if err := c.graphQL(query, nil, &result); err != nil {
		return nil, err
	}

	orgs := make([]string, len(result.Viewer.Organizations.Nodes))
	for i, node := range result.Viewer.Organizations.Nodes {
		orgs[i] = node.Login
	}
	return orgs, nil
}

func (c *Client) FetchAllPRs(orgs []string) (PRsBySection, error) {
	username, err := c.GetUsername()
	if err != nil {
		return nil, fmt.Errorf("getting username: %w", err)
	}

	result := PRsBySection{
		SectionCreated:         {},
		SectionReviewRequested: {},
		SectionAssigned:        {},
		SectionMentions:        {},
	}

	type sectionQuery struct {
		section Section
		query   string
	}

	queries := []sectionQuery{
		{SectionCreated, fmt.Sprintf("author:%s is:pr is:open", username)},
		{SectionReviewRequested, fmt.Sprintf("review-requested:%s is:pr is:open", username)},
		{SectionAssigned, fmt.Sprintf("assignee:%s is:pr is:open", username)},
		{SectionMentions, fmt.Sprintf("mentions:%s is:pr is:open", username)},
	}

	// Add org scoping
	orgFilter := ""
	if len(orgs) > 0 {
		parts := make([]string, len(orgs))
		for i, org := range orgs {
			parts[i] = "org:" + org
		}
		orgFilter = " " + strings.Join(parts, " ")
	}

	for _, sq := range queries {
		prs, err := c.searchPRs(sq.query + orgFilter)
		if err != nil {
			return nil, fmt.Errorf("fetching %s: %w", sq.section, err)
		}
		result[sq.section] = prs
	}

	return result, nil
}

func (c *Client) searchPRs(searchQuery string) ([]PullRequest, error) {
	query := `query($q: String!) {
		search(query: $q, type: ISSUE, first: 50) {
			nodes {
				... on PullRequest {
					title
					number
					url
					createdAt
					updatedAt
					isDraft
					state
					author {
						login
					}
					repository {
						nameWithOwner
					}
					labels(first: 10) {
						nodes {
							name
						}
					}
					reviewDecision
					commits(last: 1) {
						nodes {
							commit {
								statusCheckRollup {
									state
								}
							}
						}
					}
				}
			}
		}
	}`

	var result struct {
		Search struct {
			Nodes []struct {
				Title      string    `json:"title"`
				Number     int       `json:"number"`
				URL        string    `json:"url"`
				CreatedAt  time.Time `json:"createdAt"`
				UpdatedAt  time.Time `json:"updatedAt"`
				IsDraft    bool      `json:"isDraft"`
				State      string    `json:"state"`
				Author     *struct {
					Login string `json:"login"`
				} `json:"author"`
				Repository struct {
					NameWithOwner string `json:"nameWithOwner"`
				} `json:"repository"`
				Labels struct {
					Nodes []struct {
						Name string `json:"name"`
					} `json:"nodes"`
				} `json:"labels"`
				ReviewDecision string `json:"reviewDecision"`
				Commits        struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								State string `json:"state"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"nodes"`
		} `json:"search"`
	}

	vars := map[string]interface{}{"q": searchQuery}
	if err := c.graphQL(query, vars, &result); err != nil {
		return nil, err
	}

	prs := make([]PullRequest, 0, len(result.Search.Nodes))
	for _, node := range result.Search.Nodes {
		if node.Title == "" {
			continue
		}
		author := ""
		if node.Author != nil {
			author = node.Author.Login
		}
		labels := make([]string, len(node.Labels.Nodes))
		for i, l := range node.Labels.Nodes {
			labels[i] = l.Name
		}
		checksState := ""
		if len(node.Commits.Nodes) > 0 {
			rollup := node.Commits.Nodes[0].Commit.StatusCheckRollup
			if rollup != nil {
				checksState = rollup.State
			}
		}
		pr := PullRequest{
			Title:        node.Title,
			Number:       node.Number,
			Repository:   node.Repository.NameWithOwner,
			Author:       author,
			URL:          node.URL,
			CreatedAt:    node.CreatedAt,
			UpdatedAt:    node.UpdatedAt,
			IsDraft:      node.IsDraft,
			Labels:       labels,
			Status:       mapPRStatus(node.State, node.IsDraft),
			ReviewStatus: mapReviewStatus(node.ReviewDecision),
			ChecksState:  checksState,
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

// SplitOwnerRepo splits "owner/repo" into its two parts.
func SplitOwnerRepo(nameWithOwner string) (string, string) {
	parts := strings.SplitN(nameWithOwner, "/", 2)
	if len(parts) != 2 {
		return nameWithOwner, ""
	}
	return parts[0], parts[1]
}

// FetchPRDetail fetches rich detail for a single PR via GraphQL.
func (c *Client) FetchPRDetail(owner, repo string, number int) (*PRDetail, error) {
	query := `query($owner: String!, $repo: String!, $number: Int!) {
		repository(owner: $owner, name: $repo) {
			pullRequest(number: $number) {
				title
				body
				number
				url
				state
				isDraft
				mergeable
				additions
				deletions
				changedFiles
				baseRefName
				headRefName
				createdAt
				updatedAt
				mergedAt
				mergedBy { login }
				author { login }
				assignees(first: 10) { nodes { login } }
				labels(first: 10) { nodes { name } }
				reviewDecision
				reviews(last: 20) {
					nodes {
						author { login }
						state
					}
				}
				comments { totalCount }
				commits(last: 1) {
					nodes {
						commit {
							oid
							statusCheckRollup {
								contexts(first: 50) {
									nodes {
										__typename
										... on CheckRun {
											name
											conclusion
											status
											detailsUrl
										}
										... on StatusContext {
											context
											state
											targetUrl
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	var result struct {
		Repository struct {
			PullRequest struct {
				Title        string    `json:"title"`
				Body         string    `json:"body"`
				Number       int       `json:"number"`
				URL          string    `json:"url"`
				State        string    `json:"state"`
				IsDraft      bool      `json:"isDraft"`
				Mergeable    string    `json:"mergeable"`
				Additions    int       `json:"additions"`
				Deletions    int       `json:"deletions"`
				ChangedFiles int       `json:"changedFiles"`
				BaseRefName  string    `json:"baseRefName"`
				HeadRefName  string    `json:"headRefName"`
				CreatedAt    time.Time `json:"createdAt"`
				UpdatedAt    time.Time `json:"updatedAt"`
				MergedAt     *time.Time `json:"mergedAt"`
				MergedBy     *struct {
					Login string `json:"login"`
				} `json:"mergedBy"`
				Author *struct {
					Login string `json:"login"`
				} `json:"author"`
				Assignees struct {
					Nodes []struct {
						Login string `json:"login"`
					} `json:"nodes"`
				} `json:"assignees"`
				Labels struct {
					Nodes []struct {
						Name string `json:"name"`
					} `json:"nodes"`
				} `json:"labels"`
				ReviewDecision string `json:"reviewDecision"`
				Reviews        struct {
					Nodes []struct {
						Author *struct {
							Login string `json:"login"`
						} `json:"author"`
						State string `json:"state"`
					} `json:"nodes"`
				} `json:"reviews"`
				Comments struct {
					TotalCount int `json:"totalCount"`
				} `json:"comments"`
				Commits struct {
					Nodes []struct {
						Commit struct {
							Oid               string `json:"oid"`
							StatusCheckRollup *struct {
								Contexts struct {
									Nodes []struct {
										Typename   string `json:"__typename"`
										Name       string `json:"name"`       // CheckRun
										Conclusion string `json:"conclusion"` // CheckRun
										Status     string `json:"status"`     // CheckRun
										DetailsURL string `json:"detailsUrl"` // CheckRun
										Context    string `json:"context"`    // StatusContext
										State      string `json:"state"`      // StatusContext
										TargetURL  string `json:"targetUrl"`  // StatusContext
									} `json:"nodes"`
								} `json:"contexts"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	vars := map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": number,
	}
	if err := c.graphQL(query, vars, &result); err != nil {
		return nil, err
	}

	pr := result.Repository.PullRequest

	detail := &PRDetail{
		Title:          pr.Title,
		Body:           pr.Body,
		Number:         pr.Number,
		URL:            pr.URL,
		State:          pr.State,
		IsDraft:        pr.IsDraft,
		Mergeable:      pr.Mergeable,
		Additions:      pr.Additions,
		Deletions:      pr.Deletions,
		ChangedFiles:   pr.ChangedFiles,
		BaseRefName:    pr.BaseRefName,
		HeadRefName:    pr.HeadRefName,
		CreatedAt:      pr.CreatedAt,
		UpdatedAt:      pr.UpdatedAt,
		MergedAt:       pr.MergedAt,
		ReviewDecision: pr.ReviewDecision,
		CommentsCount:  pr.Comments.TotalCount,
		Repository:     owner + "/" + repo,
	}

	if len(pr.Commits.Nodes) > 0 {
		detail.HeadCommitSHA = pr.Commits.Nodes[0].Commit.Oid
	}

	if pr.Author != nil {
		detail.Author = pr.Author.Login
	}
	if pr.MergedBy != nil {
		detail.MergedBy = pr.MergedBy.Login
	}

	for _, a := range pr.Assignees.Nodes {
		detail.Assignees = append(detail.Assignees, a.Login)
	}
	for _, l := range pr.Labels.Nodes {
		detail.Labels = append(detail.Labels, l.Name)
	}

	// De-duplicate reviews by author (keep latest)
	reviewMap := make(map[string]string)
	var reviewOrder []string
	for _, r := range pr.Reviews.Nodes {
		if r.Author == nil {
			continue
		}
		if _, seen := reviewMap[r.Author.Login]; !seen {
			reviewOrder = append(reviewOrder, r.Author.Login)
		}
		reviewMap[r.Author.Login] = r.State
	}
	for _, author := range reviewOrder {
		detail.Reviews = append(detail.Reviews, ReviewEntry{Author: author, State: reviewMap[author]})
	}

	// Parse check statuses
	if len(pr.Commits.Nodes) > 0 {
		rollup := pr.Commits.Nodes[0].Commit.StatusCheckRollup
		if rollup != nil {
			for _, ctx := range rollup.Contexts.Nodes {
				var check CheckRun
				if ctx.Typename == "CheckRun" {
					check.Name = ctx.Name
					check.Status = mapCheckRunStatus(ctx.Status, ctx.Conclusion)
					check.URL = ctx.DetailsURL
				} else if ctx.Typename == "StatusContext" {
					check.Name = ctx.Context
					check.Status = mapStatusContextState(ctx.State)
					check.URL = ctx.TargetURL
				} else {
					continue
				}
				detail.Checks = append(detail.Checks, check)
			}
		}
	}

	return detail, nil
}

// FetchPRFiles fetches the file-level diffs for a PR via the REST API.
func (c *Client) FetchPRFiles(owner, repo string, number int) ([]ChangedFile, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/files?per_page=100", owner, repo, number)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status)
	}

	var rawFiles []struct {
		Filename         string `json:"filename"`
		Status           string `json:"status"`
		Additions        int    `json:"additions"`
		Deletions        int    `json:"deletions"`
		Patch            string `json:"patch"`
		PreviousFilename string `json:"previous_filename"`
	}
	if err := json.Unmarshal(body, &rawFiles); err != nil {
		return nil, fmt.Errorf("parsing files: %w", err)
	}

	files := make([]ChangedFile, len(rawFiles))
	for i, f := range rawFiles {
		files[i] = ChangedFile{
			Filename:         f.Filename,
			Status:           f.Status,
			Additions:        f.Additions,
			Deletions:        f.Deletions,
			Patch:            f.Patch,
			PreviousFilename: f.PreviousFilename,
		}
	}
	return files, nil
}

// FetchPRReviewComments fetches inline review comments for a PR via the REST API.
func (c *Client) FetchPRReviewComments(owner, repo string, number int) ([]ReviewComment, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/comments?per_page=100", owner, repo, number)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status)
	}

	var raw []struct {
		Body      string    `json:"body"`
		Path      string    `json:"path"`
		Line      *int      `json:"line"`
		Side      string    `json:"side"`
		CreatedAt time.Time `json:"created_at"`
		User      *struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing comments: %w", err)
	}

	var comments []ReviewComment
	for _, r := range raw {
		if r.Line == nil {
			continue // skip outdated comments with no line mapping
		}
		author := ""
		if r.User != nil {
			author = r.User.Login
		}
		comments = append(comments, ReviewComment{
			Author:    author,
			Body:      r.Body,
			Path:      r.Path,
			Line:      *r.Line,
			Side:      r.Side,
			CreatedAt: r.CreatedAt,
		})
	}
	return comments, nil
}

// CreateReviewComment creates a single inline review comment on a PR.
// position is the 1-based line index in the diff (first @@ line = 1).
func (c *Client) CreateReviewComment(owner, repo string, number int, commitSHA, path, body string, position int) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/comments", owner, repo, number)

	payload := map[string]interface{}{
		"body":      body,
		"commit_id": commitSHA,
		"path":      path,
		"position":  position,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling comment: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// CreatePRComment creates a general comment on a PR (not tied to a specific line).
func (c *Client) CreatePRComment(owner, repo string, number int, body string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", owner, repo, number)

	payload := map[string]string{"body": body}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling comment: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func mapCheckRunStatus(status, conclusion string) CheckStatus {
	switch strings.ToUpper(status) {
	case "COMPLETED":
		switch strings.ToUpper(conclusion) {
		case "SUCCESS":
			return CheckSuccess
		case "FAILURE", "TIMED_OUT", "ACTION_REQUIRED":
			return CheckFailure
		case "SKIPPED":
			return CheckSkipped
		case "NEUTRAL":
			return CheckNeutral
		default:
			return CheckFailure
		}
	case "IN_PROGRESS":
		return CheckInProgress
	default:
		return CheckPending
	}
}

func mapStatusContextState(state string) CheckStatus {
	switch strings.ToUpper(state) {
	case "SUCCESS":
		return CheckSuccess
	case "FAILURE", "ERROR":
		return CheckFailure
	default:
		return CheckPending
	}
}

func mapPRStatus(state string, isDraft bool) PRStatus {
	if isDraft {
		return PRStatusDraft
	}
	switch strings.ToUpper(state) {
	case "MERGED":
		return PRStatusMerged
	case "CLOSED":
		return PRStatusClosed
	default:
		return PRStatusOpen
	}
}

func mapReviewStatus(decision string) ReviewStatus {
	switch decision {
	case "APPROVED":
		return ReviewApproved
	case "CHANGES_REQUESTED":
		return ReviewChangesReq
	case "REVIEW_REQUIRED":
		return ReviewRequired
	default:
		return ReviewPending
	}
}

// ApprovePR submits an approval review on a pull request.
func (c *Client) ApprovePR(owner, repo string, number int, body string) error {
	return c.submitReview(owner, repo, number, "APPROVE", body)
}

// RequestChangesPR submits a "request changes" review on a pull request.
func (c *Client) RequestChangesPR(owner, repo string, number int, body string) error {
	return c.submitReview(owner, repo, number, "REQUEST_CHANGES", body)
}

func (c *Client) submitReview(owner, repo string, number int, event, body string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/reviews", owner, repo, number)

	payload := map[string]string{
		"event": event,
		"body":  body,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling review: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// MergePR squash-merges a pull request.
func (c *Client) MergePR(owner, repo string, number int, commitTitle string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/merge", owner, repo, number)

	payload := map[string]string{
		"merge_method": "squash",
		"commit_title": commitTitle,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling merge: %w", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
