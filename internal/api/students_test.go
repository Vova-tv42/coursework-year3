package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"coursework/internal/store"
)

const testPassword = "correct-horse"

var testDatabase *postgres.PostgresContainer

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()
	container, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("commitradar_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithSQLDriver("pgx"),
		postgres.BasicWaitStrategies(),
	)
	defer func() { _ = testcontainers.TerminateContainer(container) }()
	if err != nil {
		log.Printf("start postgres container: %v", err)
		return 1
	}

	if err := migrateAndSnapshot(ctx, container); err != nil {
		log.Print(err)
		return 1
	}
	testDatabase = container
	return m.Run()
}

func migrateAndSnapshot(ctx context.Context, container *postgres.PostgresContainer) error {
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return err
	}
	pool, err := store.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	err = store.Migrate(ctx, pool)
	pool.Close()
	if err != nil {
		return err
	}
	return container.Snapshot(ctx)
}

type testAPI struct {
	t       *testing.T
	baseURL string
	pool    *pgxpool.Pool
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, testDatabase.Restore(ctx))

	databaseURL, err := testDatabase.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := store.Open(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	server := httptest.NewServer(NewRouter(pool, false, fstest.MapFS{}))
	t.Cleanup(server.Close)

	return &testAPI{t: t, baseURL: server.URL, pool: pool}
}

type apiClient struct {
	t          *testing.T
	baseURL    string
	httpClient *http.Client
}

type apiResponse struct {
	status int
	body   []byte
}

func (testAPI *testAPI) newClient() *apiClient {
	jar, err := cookiejar.New(nil)
	require.NoError(testAPI.t, err)
	return &apiClient{t: testAPI.t, baseURL: testAPI.baseURL, httpClient: &http.Client{Jar: jar}}
}

func (client *apiClient) call(method, path string, body any) apiResponse {
	client.t.Helper()
	return client.send(client.newRequest(method, path, body))
}

func (client *apiClient) newRequest(method, path string, body any) *http.Request {
	client.t.Helper()
	var requestBody io.Reader
	if body != nil {
		encodedBody, err := json.Marshal(body)
		require.NoError(client.t, err)
		requestBody = bytes.NewReader(encodedBody)
	}

	request, err := http.NewRequest(method, client.baseURL+"/api"+path, requestBody)
	require.NoError(client.t, err)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func (client *apiClient) send(request *http.Request) apiResponse {
	client.t.Helper()
	response, err := client.httpClient.Do(request)
	require.NoError(client.t, err)
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	require.NoError(client.t, err)
	return apiResponse{status: response.StatusCode, body: body}
}

func decode[T any](t *testing.T, response apiResponse) T {
	t.Helper()
	var value T
	require.NoError(t, json.Unmarshal(response.body, &value), string(response.body))
	return value
}

func requireError(t *testing.T, response apiResponse, status int, code string) {
	t.Helper()
	require.Equal(t, status, response.status, string(response.body))
	require.Equal(t, code, decode[errorResponse](t, response).Code)
}

func (testAPI *testAPI) register(name, role string, stream *string) *apiClient {
	testAPI.t.Helper()
	client := testAPI.newClient()
	response := client.call(http.MethodPost, "/auth/register", registerRequest{
		Email:       name + "@example.com",
		Password:    testPassword,
		DisplayName: "User " + name,
		Role:        role,
		Stream:      stream,
	})
	require.Equal(testAPI.t, http.StatusCreated, response.status, string(response.body))
	return client
}

func (client *apiClient) addStudent(fullName, stream string) studentOverview {
	client.t.Helper()
	response := client.call(http.MethodPost, "/students", studentInput{
		FullName: fullName,
		Stream:   stream,
		CloneURL: "https://github.com/student/" + fullName + ".git",
	})
	require.Equal(client.t, http.StatusCreated, response.status, string(response.body))
	return decode[studentOverview](client.t, response)
}

func (client *apiClient) listStudentNames(path string) []string {
	client.t.Helper()
	response := client.call(http.MethodGet, path, nil)
	require.Equal(client.t, http.StatusOK, response.status, string(response.body))

	names := []string{}
	for _, student := range decode[[]studentOverview](client.t, response) {
		names = append(names, student.FullName)
	}
	return names
}

func streamOf(stream string) *string {
	return &stream
}

func TestRegisterValidatesInput(t *testing.T) {
	testAPI := newTestAPI(t)
	valid := registerRequest{Email: "anna.k@example.com", Password: testPassword, DisplayName: "Anna", Role: roleSupervisor}

	testCases := map[string]func(request *registerRequest){
		"email without at sign":      func(request *registerRequest) { request.Email = "anna.example.com" },
		"email with display name":    func(request *registerRequest) { request.Email = "anna <anna@example.com>" },
		"email over length limit":    func(request *registerRequest) { request.Email = strings.Repeat("a", 250) + "@example.com" },
		"short password":             func(request *registerRequest) { request.Password = "1234567" },
		"password over bcrypt limit": func(request *registerRequest) { request.Password = string(make([]byte, 73)) },
		"empty display name":         func(request *registerRequest) { request.DisplayName = "" },
		"unknown role":               func(request *registerRequest) { request.Role = "admin" },
		"coordinator without stream": func(request *registerRequest) { request.Role = roleCoordinator },
		"supervisor with stream":     func(request *registerRequest) { request.Stream = streamOf("bachelor") },
		"unknown stream": func(request *registerRequest) {
			request.Role = roleCoordinator
			request.Stream = streamOf("phd")
		},
	}
	for name, breakRequest := range testCases {
		request := valid
		breakRequest(&request)
		response := testAPI.newClient().call(http.MethodPost, "/auth/register", request)
		require.Equal(t, http.StatusBadRequest, response.status, name)
		require.Equal(t, "invalid_input", decode[errorResponse](t, response).Code, name)
	}

	response := testAPI.newClient().call(http.MethodPost, "/auth/register", "not an object")
	requireError(t, response, http.StatusBadRequest, "bad_request")
}

func TestRegisterSignsInAndRejectsTakenEmail(t *testing.T) {
	testAPI := newTestAPI(t)
	coordinator := testAPI.register("coord", roleCoordinator, streamOf("master"))

	response := coordinator.call(http.MethodGet, "/auth/me", nil)
	require.Equal(t, http.StatusOK, response.status)
	me := decode[user](t, response)
	require.Equal(t, "coord@example.com", me.Email)
	require.Equal(t, roleCoordinator, me.Role)
	require.Equal(t, streamOf("master"), me.Stream)
	require.NotContains(t, string(response.body), "password")

	response = testAPI.newClient().call(http.MethodPost, "/auth/register", registerRequest{
		Email: " Coord@Example.com ", Password: testPassword, DisplayName: "Other", Role: roleSupervisor,
	})
	requireError(t, response, http.StatusConflict, "email_taken")
}

func TestLoginAndLogout(t *testing.T) {
	testAPI := newTestAPI(t)
	testAPI.register("anna", roleSupervisor, nil)
	client := testAPI.newClient()

	requireError(t, client.call(http.MethodGet, "/auth/me", nil), http.StatusUnauthorized, "unauthorized")
	requireError(t, client.call(http.MethodGet, "/students", nil), http.StatusUnauthorized, "unauthorized")

	wrongPassword := loginRequest{Email: "anna@example.com", Password: "wrong-password"}
	requireError(t, client.call(http.MethodPost, "/auth/login", wrongPassword), http.StatusUnauthorized, "invalid_credentials")
	unknownEmail := loginRequest{Email: "nobody@example.com", Password: testPassword}
	requireError(t, client.call(http.MethodPost, "/auth/login", unknownEmail), http.StatusUnauthorized, "invalid_credentials")

	response := client.call(http.MethodPost, "/auth/login", loginRequest{Email: "ANNA@example.com", Password: testPassword})
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	require.Equal(t, "anna@example.com", decode[user](t, response).Email)
	require.Equal(t, http.StatusOK, client.call(http.MethodGet, "/auth/me", nil).status)

	require.Equal(t, http.StatusNoContent, client.call(http.MethodPost, "/auth/logout", nil).status)
	requireError(t, client.call(http.MethodGet, "/auth/me", nil), http.StatusUnauthorized, "unauthorized")
}

func TestStudentListsAreScopedByRole(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	boris := testAPI.register("boris", roleSupervisor, nil)
	bachelorCoordinator := testAPI.register("bachelors", roleCoordinator, streamOf("bachelor"))
	masterCoordinator := testAPI.register("masters", roleCoordinator, streamOf("master"))
	repoManager := testAPI.register("manager", roleRepoManager, nil)

	anna.addStudent("Alice", "bachelor")
	anna.addStudent("Bob", "master")
	boris.addStudent("Carol", "bachelor")

	require.Equal(t, []string{"Alice", "Bob"}, anna.listStudentNames("/students"))
	require.Equal(t, []string{"Carol"}, boris.listStudentNames("/students"))
	require.Equal(t, []string{"Alice", "Carol"}, bachelorCoordinator.listStudentNames("/students"))
	require.Equal(t, []string{"Bob"}, masterCoordinator.listStudentNames("/students"))
	require.Equal(t, []string{"Alice", "Bob", "Carol"}, repoManager.listStudentNames("/students"))

	require.Equal(t, []string{"Bob"}, anna.listStudentNames("/students?stream=master"))
	require.Equal(t, []string{"Alice", "Carol"}, repoManager.listStudentNames("/students?stream=bachelor"))
	require.Empty(t, repoManager.listStudentNames("/students?stream=phd"))
}

func TestStudentOverviewFields(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	repoManager := testAPI.register("manager", roleRepoManager, nil)
	created := anna.addStudent("Alice", "bachelor")

	require.Equal(t, "Alice", created.FullName)
	require.Equal(t, "bachelor", created.Stream)
	require.Equal(t, "User anna", created.SupervisorName)
	require.True(t, created.CanEdit)
	require.Equal(t, created.ID, created.Repository.StudentID)
	require.Equal(t, "https://github.com/student/Alice.git", created.Repository.CloneURL)
	require.Equal(t, "never", created.Repository.Status)
	require.Nil(t, created.Repository.LastCommitAt)

	response := repoManager.call(http.MethodGet, fmt.Sprintf("/students/%d", created.ID), nil)
	require.Equal(t, http.StatusOK, response.status)
	require.False(t, decode[studentOverview](t, response).CanEdit)
}

func TestForeignStudentAnswersNotFound(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	boris := testAPI.register("boris", roleSupervisor, nil)
	masterCoordinator := testAPI.register("masters", roleCoordinator, streamOf("master"))
	bachelorCoordinator := testAPI.register("bachelors", roleCoordinator, streamOf("bachelor"))
	student := anna.addStudent("Alice", "bachelor")
	studentPath := fmt.Sprintf("/students/%d", student.ID)
	update := studentInput{FullName: "Changed", Stream: "master", CloneURL: "git@github.com:other/repo.git"}

	requireError(t, boris.call(http.MethodGet, studentPath, nil), http.StatusNotFound, "not_found")
	requireError(t, boris.call(http.MethodPut, studentPath, update), http.StatusNotFound, "not_found")
	requireError(t, boris.call(http.MethodDelete, studentPath, nil), http.StatusNotFound, "not_found")
	requireError(t, masterCoordinator.call(http.MethodGet, studentPath, nil), http.StatusNotFound, "not_found")
	requireError(t, anna.call(http.MethodGet, "/students/999999", nil), http.StatusNotFound, "not_found")
	requireError(t, anna.call(http.MethodGet, "/students/abc", nil), http.StatusNotFound, "not_found")

	// A coordinator sees the student of their stream but cannot change it.
	require.Equal(t, http.StatusOK, bachelorCoordinator.call(http.MethodGet, studentPath, nil).status)
	requireError(t, bachelorCoordinator.call(http.MethodPut, studentPath, update), http.StatusNotFound, "not_found")
	requireError(t, bachelorCoordinator.call(http.MethodDelete, studentPath, nil), http.StatusNotFound, "not_found")

	response := anna.call(http.MethodGet, studentPath, nil)
	require.Equal(t, http.StatusOK, response.status)
	require.Equal(t, "Alice", decode[studentOverview](t, response).FullName)
}

func TestOnlySupervisorsAddStudents(t *testing.T) {
	testAPI := newTestAPI(t)
	coordinator := testAPI.register("bachelors", roleCoordinator, streamOf("bachelor"))
	repoManager := testAPI.register("manager", roleRepoManager, nil)
	input := studentInput{FullName: "Alice", Stream: "bachelor", CloneURL: "https://github.com/a/b.git"}

	requireError(t, coordinator.call(http.MethodPost, "/students", input), http.StatusForbidden, "forbidden")
	requireError(t, repoManager.call(http.MethodPost, "/students", input), http.StatusForbidden, "forbidden")
	require.Empty(t, repoManager.listStudentNames("/students"))
}

func TestStudentInputValidation(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)

	validCloneURLs := []string{
		"https://github.com/student/thesis.git",
		"http://git.example.com/thesis",
		"git://example.com/thesis.git",
		"git@github.com:student/thesis.git",
	}
	for _, cloneURL := range validCloneURLs {
		input := studentInput{FullName: "Alice", Stream: "bachelor", CloneURL: cloneURL}
		require.Equal(t, http.StatusCreated, anna.call(http.MethodPost, "/students", input).status, cloneURL)
	}

	invalidInputs := []studentInput{
		{FullName: "", Stream: "bachelor", CloneURL: "https://github.com/a/b.git"},
		{FullName: "Alice", Stream: "phd", CloneURL: "https://github.com/a/b.git"},
		{FullName: "Alice", Stream: "bachelor", CloneURL: "ftp://example.com/a.git"},
		{FullName: "Alice", Stream: "bachelor", CloneURL: "https://github.com"},
		{FullName: "Alice", Stream: "bachelor", CloneURL: "/home/student/repo"},
		{FullName: "Alice", Stream: "bachelor", CloneURL: ""},
	}
	for _, input := range invalidInputs {
		response := anna.call(http.MethodPost, "/students", input)
		requireError(t, response, http.StatusBadRequest, "invalid_input")
	}
}

func TestUpdateStudentResetsAnalysisOnlyWhenCloneURLChanges(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student := anna.addStudent("Alice", "bachelor")
	studentPath := fmt.Sprintf("/students/%d", student.ID)
	testAPI.fakeFinishedAnalysis(student.Repository.ID)

	sameURL := studentInput{FullName: "Alice Smith", Stream: "master", CloneURL: student.Repository.CloneURL}
	response := anna.call(http.MethodPut, studentPath, sameURL)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	updated := decode[studentOverview](t, response)
	require.Equal(t, "Alice Smith", updated.FullName)
	require.Equal(t, "master", updated.Stream)
	require.Equal(t, "done", updated.Repository.Status)
	require.Equal(t, 1, testAPI.countRows("commits", student.Repository.ID))

	newURL := studentInput{FullName: "Alice Smith", Stream: "master", CloneURL: "git@gitlab.com:alice/thesis.git"}
	response = anna.call(http.MethodPut, studentPath, newURL)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	updated = decode[studentOverview](t, response)
	require.Equal(t, "git@gitlab.com:alice/thesis.git", updated.Repository.CloneURL)
	require.Equal(t, "never", updated.Repository.Status)
	require.Nil(t, updated.Repository.DefaultBranch)
	require.Nil(t, updated.Repository.LastCommitAt)
	for _, table := range []string{"commits", "file_changes", "snapshot_files", "rapid_series"} {
		require.Zero(t, testAPI.countRows(table, student.Repository.ID), table)
	}
}

func TestDeleteStudent(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	student := anna.addStudent("Alice", "bachelor")
	studentPath := fmt.Sprintf("/students/%d", student.ID)

	require.Equal(t, http.StatusNoContent, anna.call(http.MethodDelete, studentPath, nil).status)
	requireError(t, anna.call(http.MethodGet, studentPath, nil), http.StatusNotFound, "not_found")
	requireError(t, anna.call(http.MethodDelete, studentPath, nil), http.StatusNotFound, "not_found")
	require.Zero(t, testAPI.countRows("repositories", student.Repository.ID))
}

func TestCrossOriginMutationIsRejected(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	input := studentInput{FullName: "Alice", Stream: "bachelor", CloneURL: "https://github.com/a/b.git"}

	crossSiteRequest := anna.newRequest(http.MethodPost, "/students", input)
	crossSiteRequest.Header.Set("Sec-Fetch-Site", "cross-site")
	require.Equal(t, http.StatusForbidden, anna.send(crossSiteRequest).status)

	foreignOriginRequest := anna.newRequest(http.MethodPost, "/students", input)
	foreignOriginRequest.Header.Set("Origin", "https://evil.example")
	require.Equal(t, http.StatusForbidden, anna.send(foreignOriginRequest).status)

	sameOriginRequest := anna.newRequest(http.MethodPost, "/students", input)
	sameOriginRequest.Header.Set("Sec-Fetch-Site", "same-origin")
	require.Equal(t, http.StatusCreated, anna.send(sameOriginRequest).status)
	require.Equal(t, []string{"Alice"}, anna.listStudentNames("/students"))
}

func (testAPI *testAPI) fakeFinishedAnalysis(repositoryID int64) {
	testAPI.t.Helper()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	statements := []string{
		`UPDATE repositories SET status = 'done', default_branch = 'main', last_analyzed_head = '` + sha + `',
			last_commit_at = now(), analysis_finished_at = now() WHERE id = $1`,
		`INSERT INTO commits (repository_id, sha, author_name, author_email, authored_at, committed_at, message, is_merge, lines_added, lines_deleted)
			VALUES ($1, '` + sha + `', 'Alice', 'alice@example.com', now(), now(), 'Initial commit', false, 3, 0)`,
		`INSERT INTO file_changes (repository_id, sha, path, lines_added, lines_deleted, is_binary)
			VALUES ($1, '` + sha + `', 'main.go', 3, 0, false)`,
		`INSERT INTO snapshot_files (repository_id, path, size_bytes, is_binary, category, code_lines, comment_lines)
			VALUES ($1, 'main.go', 30, false, 'code', 3, 0)`,
		`INSERT INTO rapid_series (repository_id, commit_shas, started_at, ended_at)
			VALUES ($1, ARRAY['` + sha + `', '` + sha + `', '` + sha + `'], now(), now())`,
	}
	for _, statement := range statements {
		_, err := testAPI.pool.Exec(testAPI.t.Context(), statement, repositoryID)
		require.NoError(testAPI.t, err)
	}
}

func (testAPI *testAPI) countRows(table string, repositoryID int64) int {
	testAPI.t.Helper()
	idColumn := "repository_id"
	if table == "repositories" {
		idColumn = "id"
	}

	var count int
	query := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = $1", table, idColumn)
	require.NoError(testAPI.t, testAPI.pool.QueryRow(testAPI.t.Context(), query, repositoryID).Scan(&count))
	return count
}
