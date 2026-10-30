package worker

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-github/v92/github"
)

const (
	githubHost           = "github.com"
	githubRequestTimeout = 15 * time.Second
)

// HostingMetadata is stored in repositories.hosting_metadata and returned by the API as is
type HostingMetadata struct {
	Available     bool       `json:"available"`
	DefaultBranch string     `json:"defaultBranch,omitempty"`
	PushedAt      *time.Time `json:"pushedAt,omitempty"`
	SizeKB        int        `json:"sizeKb,omitempty"`
}

type GitHubClient struct {
	client *github.Client
}

func NewGitHubClient(token string, extraOptions ...github.ClientOptionsFunc) (*GitHubClient, error) {
	options := []github.ClientOptionsFunc{github.WithTimeout(githubRequestTimeout)}
	if token != "" {
		options = append(options, github.WithAuthToken(token))
	}

	client, err := github.NewClient(append(options, extraOptions...)...)
	if err != nil {
		return nil, err
	}

	return &GitHubClient{client: client}, nil
}

// Metadata returns nil for repositories outside GitHub; a failed request gives {available: false}
func (c *GitHubClient) Metadata(ctx context.Context, cloneURL string) *HostingMetadata {
	owner, name, isGitHubRepository := parseGitHubRepository(cloneURL)
	if !isGitHubRepository {
		return nil
	}

	repository, _, err := c.client.Repositories.Get(ctx, owner, name)
	if err != nil {
		slog.Warn("github metadata unavailable", "owner", owner, "repository", name, "error", err)
		return &HostingMetadata{Available: false}
	}

	metadata := &HostingMetadata{
		Available:     true,
		DefaultBranch: repository.GetDefaultBranch(),
		SizeKB:        repository.GetSize(),
	}

	if repository.PushedAt != nil {
		pushedAt := repository.PushedAt.UTC()
		metadata.PushedAt = &pushedAt
	}

	return metadata
}

// parseGitHubRepository accepts https://github.com/owner/name(.git) and git@github.com:owner/name(.git)
func parseGitHubRepository(cloneURL string) (owner, name string, ok bool) {
	var host, repositoryPath string
	if sshPath, isSSHURL := strings.CutPrefix(cloneURL, "git@"+githubHost+":"); isSSHURL {
		host, repositoryPath = githubHost, sshPath
	} else {
		parsedURL, err := url.Parse(cloneURL)
		if err != nil {
			return "", "", false
		}

		host, repositoryPath = parsedURL.Host, parsedURL.Path
	}

	if !strings.EqualFold(host, githubHost) {
		return "", "", false
	}

	repositoryPath = strings.TrimSuffix(strings.Trim(repositoryPath, "/"), ".git")
	owner, name, found := strings.Cut(repositoryPath, "/")
	isOwnerAndName := found && owner != "" && name != "" && !strings.Contains(name, "/")
	return owner, name, isOwnerAndName
}
