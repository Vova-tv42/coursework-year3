package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"coursework/internal/metrics"
)

const bachelorPracticeName = "Bachelor practice"

func (client *apiClient) calendarPeriods() []metrics.CalendarPeriod {
	client.t.Helper()
	response := client.call(http.MethodGet, "/calendar", nil)
	require.Equal(client.t, http.StatusOK, response.status, string(response.body))

	return decode[[]metrics.CalendarPeriod](client.t, response)
}

func (client *apiClient) periodID(name string) int64 {
	client.t.Helper()
	for _, period := range client.calendarPeriods() {
		if period.Name == name {
			return period.ID
		}
	}

	require.FailNow(client.t, "calendar period not found", name)
	return 0
}

func (client *apiClient) setPeriodDates(name, startDate, endDate string) {
	client.t.Helper()
	response := client.call(http.MethodPut, fmt.Sprintf("/calendar/%d", client.periodID(name)),
		calendarPeriodInput{Name: name, StartDate: &startDate, EndDate: &endDate})
	require.Equal(client.t, http.StatusOK, response.status, string(response.body))
}

func (client *apiClient) periodActivity(repositoryID int64, name string) metrics.PeriodActivitySummary {
	client.t.Helper()
	response := client.call(http.MethodGet, fmt.Sprintf("/repos/%d/activity", repositoryID), nil)
	require.Equal(client.t, http.StatusOK, response.status, string(response.body))

	for _, period := range decode[activityResponse](client.t, response).Periods {
		if period.Name == name {
			return period
		}
	}

	require.FailNow(client.t, "period missing in activity", name)
	return metrics.PeriodActivitySummary{}
}

func TestCalendarHasDefaultPeriods(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)

	periods := anna.calendarPeriods()

	names := []string{}
	for _, period := range periods {
		names = append(names, period.Name)
	}
	require.Equal(t, []string{"Start of thesis work", bachelorPracticeName, "Master practice"}, names)
	require.Equal(t, "2027-04-01", *periods[1].StartDate)
	require.Equal(t, "2027-05-31", *periods[1].EndDate)
	require.Nil(t, periods[2].StartDate)
	require.Nil(t, periods[2].EndDate)
}

func TestOnlyCoordinatorsAndRepoManagersEditCalendar(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	coordinator := testAPI.register("bachelors", roleCoordinator, streamOf("bachelor"))
	manager := testAPI.register("manager", roleRepoManager, nil)
	practicePath := fmt.Sprintf("/calendar/%d", anna.periodID(bachelorPracticeName))
	startDate, endDate := "2027-04-05", "2027-05-20"
	input := calendarPeriodInput{Name: "Spring practice", StartDate: &startDate, EndDate: &endDate}

	requireError(t, anna.call(http.MethodPut, practicePath, input), http.StatusForbidden, "forbidden")

	for _, editor := range []*apiClient{coordinator, manager} {
		response := editor.call(http.MethodPut, practicePath, input)
		require.Equal(t, http.StatusOK, response.status, string(response.body))
		period := decode[metrics.CalendarPeriod](t, response)
		require.Equal(t, "Spring practice", period.Name)
		require.Equal(t, metrics.PeriodKindPractice, period.Kind)
		require.Equal(t, "bachelor", period.Stream)
		require.Equal(t, startDate, *period.StartDate)
		require.Equal(t, endDate, *period.EndDate)
	}

	requireError(t, manager.call(http.MethodPut, "/calendar/999999", input), http.StatusNotFound, "not_found")
	requireError(t, manager.call(http.MethodPut, "/calendar/abc", input), http.StatusNotFound, "not_found")
}

func TestCalendarPeriodValidation(t *testing.T) {
	testAPI := newTestAPI(t)
	manager := testAPI.register("manager", roleRepoManager, nil)
	practicePath := fmt.Sprintf("/calendar/%d", manager.periodID(bachelorPracticeName))
	dateOf := func(date string) *string { return &date }

	invalidInputs := map[string]struct {
		path  string
		input calendarPeriodInput
	}{
		"empty name":        {practicePath, calendarPeriodInput{Name: ""}},
		"long name":         {practicePath, calendarPeriodInput{Name: strings.Repeat("a", 101)}},
		"only start date":   {practicePath, calendarPeriodInput{Name: "Practice", StartDate: dateOf("2027-04-01")}},
		"end before start":  {practicePath, calendarPeriodInput{Name: "Practice", StartDate: dateOf("2027-04-10"), EndDate: dateOf("2027-04-01")}},
		"wrong date format": {practicePath, calendarPeriodInput{Name: "Practice", StartDate: dateOf("01.04.2027"), EndDate: dateOf("2027-05-31")}},
		"impossible date":   {practicePath, calendarPeriodInput{Name: "Practice", StartDate: dateOf("2027-02-30"), EndDate: dateOf("2027-05-31")}},
	}

	for name, invalid := range invalidInputs {
		t.Run(name, func(t *testing.T) {
			requireError(t, manager.call(http.MethodPut, invalid.path, invalid.input), http.StatusBadRequest, "invalid_input")
		})
	}

	response := manager.call(http.MethodPut, practicePath, calendarPeriodInput{Name: "Practice"})
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	cleared := decode[metrics.CalendarPeriod](t, response)
	require.Nil(t, cleared.StartDate)
	require.Nil(t, cleared.EndDate)
}

func TestPeriodActivityComparesKyivDates(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	manager := testAPI.register("manager", roleRepoManager, nil)
	student, _ := testAPI.addAnalyzedStudent(anna, "Alice", "kyiv_boundary")

	// The fixture commits at 20:30 and 23:30 UTC on October 5, which is October 6 in Kyiv for the second one
	manager.setPeriodDates(bachelorPracticeName, "2026-10-01", "2026-10-05")
	practice := anna.periodActivity(student.Repository.ID, bachelorPracticeName)
	require.Equal(t, 1, practice.Commits)
	require.False(t, practice.NoActivitySignal)

	manager.setPeriodDates(bachelorPracticeName, "2026-10-06", "2026-10-31")
	practice = anna.periodActivity(student.Repository.ID, bachelorPracticeName)
	require.Equal(t, 1, practice.Commits)
	require.False(t, practice.NoActivitySignal)
}

func TestNoPracticeActivitySignal(t *testing.T) {
	testAPI := newTestAPI(t)
	anna := testAPI.register("anna", roleSupervisor, nil)
	manager := testAPI.register("manager", roleRepoManager, nil)
	alice, _ := testAPI.addAnalyzedStudent(anna, "Alice", "kyiv_boundary")
	anna.addStudent("Bob", "bachelor")
	carol := anna.addStudent("Carol", "master")
	testAPI.addAnalyzedStudent(anna, "Dan", "empty")

	manager.setPeriodDates(bachelorPracticeName, "2099-04-01", "2099-05-31")
	require.False(t, anna.periodActivity(alice.Repository.ID, bachelorPracticeName).NoActivitySignal)
	require.Equal(t, []string{}, anna.listStudentNames("/students?noPracticeActivity=true"))

	manager.setPeriodDates(bachelorPracticeName, "2026-10-07", "2026-10-31")
	require.True(t, anna.periodActivity(alice.Repository.ID, bachelorPracticeName).NoActivitySignal)
	require.Equal(t, []string{"Alice", "Dan"}, anna.listStudentNames("/students?noPracticeActivity=true"))
	require.Equal(t, []string{"Bob", "Carol"}, anna.listStudentNames("/students?noPracticeActivity=false"))

	response := anna.call(http.MethodGet, fmt.Sprintf("/repos/%d/activity", carol.Repository.ID), nil)
	require.Equal(t, http.StatusOK, response.status, string(response.body))
	for _, period := range decode[activityResponse](t, response).Periods {
		require.NotEqual(t, bachelorPracticeName, period.Name, "a master student sees only all-stream and master periods")
	}

	requireError(t, anna.call(http.MethodGet, "/students?noPracticeActivity=maybe", nil), http.StatusBadRequest, "invalid_input")
}
