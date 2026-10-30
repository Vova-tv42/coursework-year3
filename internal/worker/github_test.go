package worker

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/stretchr/testify/require"
)

const testGitHubToken = "test-token"

func newMockGitHub(t *testing.T, handler http.HandlerFunc) *GitHubClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// go-github needs the trailing slash on the base URL
	baseURL := server.URL + "/"
	client, err := NewGitHubClient(testGitHubToken, github.WithURLs(&baseURL, nil))
	require.NoError(t, err)
	return client
}

func TestGitHubMetadataOfExistingRepository(t *testing.T) {
	client := newMockGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		isExpectedRequest := r.URL.Path == "/repos/student/thesis" && r.Header.Get("Authorization") == "Bearer "+testGitHubToken
		if !isExpectedRequest {
			http.Error(w, "unexpected request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"default_branch": "develop", "pushed_at": "2026-10-01T12:30:00Z", "size": 2048}`))
	})

	metadata := client.Metadata(t.Context(), "https://github.com/student/thesis.git")

	pushedAt := time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC)
	require.Equal(t, &HostingMetadata{Available: true, DefaultBranch: "develop", PushedAt: &pushedAt, SizeKB: 2048}, metadata)
}

func TestGitHubErrorsGiveUnavailableMetadata(t *testing.T) {
	testCases := map[string]http.HandlerFunc{
		"not found": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message": "Not Found"}`, http.StatusNotFound)
		},
		"rate limit": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
			http.Error(w, `{"message": "API rate limit exceeded"}`, http.StatusForbidden)
		},
	}

	for name, handler := range testCases {
		t.Run(name, func(t *testing.T) {
			client := newMockGitHub(t, handler)
			metadata := client.Metadata(t.Context(), "git@github.com:student/thesis.git")
			require.Equal(t, &HostingMetadata{Available: false}, metadata)
		})
	}
}

func TestMetadataIsSkippedOutsideGitHub(t *testing.T) {
	client := newMockGitHub(t, func(http.ResponseWriter, *http.Request) {
		t.Error("GitHub must not be called for other hosts")
	})

	require.Nil(t, client.Metadata(t.Context(), "https://gitlab.com/student/thesis.git"))
	require.Nil(t, client.Metadata(t.Context(), "/srv/git/thesis"))
}

func TestParseGitHubRepository(t *testing.T) {
	testCases := []struct {
		cloneURL     string
		owner, name  string
		isRepository bool
	}{
		{cloneURL: "https://github.com/student/thesis.git", owner: "student", name: "thesis", isRepository: true},
		{cloneURL: "https://GitHub.com/student/thesis/", owner: "student", name: "thesis", isRepository: true},
		{cloneURL: "git@github.com:student/thesis.git", owner: "student", name: "thesis", isRepository: true},
		{cloneURL: "https://github.com/student", isRepository: false},
		{cloneURL: "https://github.com/student/thesis/tree/main", isRepository: false},
		{cloneURL: "https://gitlab.com/student/thesis.git", isRepository: false},
		{cloneURL: "git@gitlab.com:student/thesis.git", isRepository: false},
	}

	for _, testCase := range testCases {
		owner, name, isRepository := parseGitHubRepository(testCase.cloneURL)
		require.Equal(t, testCase.isRepository, isRepository, testCase.cloneURL)
		if testCase.isRepository {
			require.Equal(t, testCase.owner, owner, testCase.cloneURL)
			require.Equal(t, testCase.name, name, testCase.cloneURL)
		}
	}
}
