package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

const (
	defaultPort = "8080"
)

// getCalendarURL returns the calendar URL from environment variable
// Returns an error if CALENDAR_URL is not set
func getCalendarURL() (string, error) {
	url := os.Getenv("CALENDAR_URL")
	if url == "" {
		return "", fmt.Errorf("CALENDAR_URL environment variable is required")
	}
	return url, nil
}

// TimeRange represents a start and end time for filtering
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// FilterRequest represents the request body for filtering
type FilterRequest struct {
	TimeRanges []TimeRange `json:"time_ranges"`
}

// parseTimeRangesFromQuery parses time ranges from query parameters
// Supports two formats:
// 1. ranges=HH:MM-HH:MM,HH:MM-HH:MM (comma-separated list of start-end pairs)
// 2. start=HH:MM&end=HH:MM&start=HH:MM&end=HH:MM (repeating pairs)
// Timezone can be specified via tz parameter (e.g., tz=America/New_York) or defaults to local time
func parseTimeRangesFromQuery(r *http.Request) ([]TimeRange, *time.Location, error) {
	// Get timezone from query parameter or default to local time
	loc := time.UTC
	if tzParam := r.URL.Query().Get("tz"); tzParam != "" {
		var err error
		loc, err = time.LoadLocation(tzParam)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid timezone: %s (error: %w)", tzParam, err)
		}
	}

	// Try the simpler ranges format first: ranges=09:00-10:00,14:00-15:00
	if rangesParam := r.URL.Query().Get("ranges"); rangesParam != "" {
		ranges, err := parseRangesList(rangesParam, loc)
		return ranges, loc, err
	}

	// Fall back to start/end pairs format
	startTimes := r.URL.Query()["start"]
	endTimes := r.URL.Query()["end"]

	if len(startTimes) != len(endTimes) {
		return nil, nil, fmt.Errorf("mismatched start/end time pairs")
	}

	var ranges []TimeRange
	for i := 0; i < len(startTimes); i++ {
		start, err := parseTimeOfDay(startTimes[i], loc)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start time %s: %w", startTimes[i], err)
		}
		end, err := parseTimeOfDay(endTimes[i], loc)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end time %s: %w", endTimes[i], err)
		}
		ranges = append(ranges, TimeRange{Start: start, End: end})
	}

	return ranges, loc, nil
}

// parseRangesList parses a comma-separated list of time ranges
// Format: "09:00-10:00,14:00-15:00" or "09:00-10:00, 14:00-15:00"
func parseRangesList(rangesStr string, loc *time.Location) ([]TimeRange, error) {
	var ranges []TimeRange
	
	// Split by comma
	rangeStrings := strings.Split(rangesStr, ",")
	
	for _, rangeStr := range rangeStrings {
		rangeStr = strings.TrimSpace(rangeStr)
		if rangeStr == "" {
			continue
		}
		
		// Split by dash to get start and end
		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range format: %s (expected HH:MM-HH:MM)", rangeStr)
		}
		
		start, err := parseTimeOfDay(strings.TrimSpace(parts[0]), loc)
		if err != nil {
			return nil, fmt.Errorf("invalid start time in range %s: %w", rangeStr, err)
		}
		
		end, err := parseTimeOfDay(strings.TrimSpace(parts[1]), loc)
		if err != nil {
			return nil, fmt.Errorf("invalid end time in range %s: %w", rangeStr, err)
		}
		
		ranges = append(ranges, TimeRange{Start: start, End: end})
	}
	
	return ranges, nil
}

// parseTimeOfDay parses a time string in HH:MM format in the specified timezone
func parseTimeOfDay(timeStr string, loc *time.Location) (time.Time, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format, expected HH:MM")
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, fmt.Errorf("invalid hour: %s", parts[0])
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("invalid minute: %s", parts[1])
	}

	// Use today's date as a base, but we'll compare only time components
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc), nil
}

// eventMatchesExactRange checks if an event has exact start/end times matching any filter range
// Events are filtered out if their start time matches the filter start time and end time matches the filter end time
// Filter ranges are treated as daily recurring blocks (e.g., 09:00-10:00 matches events starting at 09:00 and ending at 10:00 on any day)
// Event times are converted to the filter timezone before comparison
func eventMatchesExactRange(eventStart, eventEnd time.Time, filterRanges []TimeRange, filterLoc *time.Location) bool {
	// Convert event times to the filter timezone
	eventStartLocal := eventStart.In(filterLoc)
	eventEndLocal := eventEnd.In(filterLoc)
	
	// Extract time components from event (in filter timezone)
	eventStartHour := eventStartLocal.Hour()
	eventStartMinute := eventStartLocal.Minute()
	eventEndHour := eventEndLocal.Hour()
	eventEndMinute := eventEndLocal.Minute()

	// Check if event matches any filter range exactly
	for _, filterRange := range filterRanges {
		filterStartHour := filterRange.Start.Hour()
		filterStartMinute := filterRange.Start.Minute()
		filterEndHour := filterRange.End.Hour()
		filterEndMinute := filterRange.End.Minute()

		// Check if event start/end times match filter start/end times exactly
		if eventStartHour == filterStartHour &&
			eventStartMinute == filterStartMinute &&
			eventEndHour == filterEndHour &&
			eventEndMinute == filterEndMinute {
			return true
		}
	}
	return false
}

// fetchCalendar fetches the ICS calendar from Google
func fetchCalendar() ([]byte, error) {
	calendarURL, err := getCalendarURL()
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(calendarURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch calendar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

// filterCalendar filters events from the calendar based on time ranges
// Returns the filtered calendar data, original event count, and filtered event count
func filterCalendar(icsData []byte, filterRanges []TimeRange, filterLoc *time.Location) ([]byte, int, int, error) {
	cal, err := ics.ParseCalendar(strings.NewReader(string(icsData)))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to parse calendar: %w", err)
	}

	// Create a new calendar with filtered events
	filteredCal := ics.NewCalendar()
	
	// Copy all calendar properties from original calendar
	filteredCal.CalendarProperties = cal.CalendarProperties

	originalCount := len(cal.Events())
	filteredCount := 0

	// Filter events
	for _, event := range cal.Events() {
		eventStart, err := event.GetStartAt()
		if err != nil {
			log.Printf("Warning: failed to get event start time: %v", err)
			continue
		}

		eventEnd, err := event.GetEndAt()
		if err != nil {
			log.Printf("Warning: failed to get event end time: %v", err)
			continue
		}

		// If event matches any filter range exactly, skip it
		if eventMatchesExactRange(eventStart, eventEnd, filterRanges, filterLoc) {
			continue
		}

		// Add event to filtered calendar
		filteredCal.AddVEvent(event)
		filteredCount++
	}

	// Serialize filtered calendar
	return []byte(filteredCal.Serialize()), originalCount, filteredCount, nil
}

// handleFilter handles the /filter endpoint
func handleFilter(w http.ResponseWriter, r *http.Request) {
	var filterRanges []TimeRange
	var filterLoc *time.Location = time.Local

	// Try to parse from JSON body first
	if r.Method == http.MethodPost {
		var req FilterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			filterRanges = req.TimeRanges
			// For JSON, use local timezone by default
			filterLoc = time.Local
		}
	}

	// If no JSON body or parsing failed, try query parameters
	if len(filterRanges) == 0 {
		var err error
		filterRanges, filterLoc, err = parseTimeRangesFromQuery(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid filter parameters: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Fetch calendar
	icsData, err := fetchCalendar()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch calendar: %v", err), http.StatusInternalServerError)
		return
	}

	// If no ranges, return original calendar and log count
	if len(filterRanges) == 0 {
		// Parse to get event count
		cal, err := ics.ParseCalendar(strings.NewReader(string(icsData)))
		if err == nil {
			eventCount := len(cal.Events())
			log.Printf("[%s] Request: no filters applied, returned %d events", r.RemoteAddr, eventCount)
		}
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Write(icsData)
		return
	}

	// Filter calendar
	filteredData, originalCount, filteredCount, err := filterCalendar(icsData, filterRanges, filterLoc)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to filter calendar: %v", err), http.StatusInternalServerError)
		return
	}

	// Log event counts
	log.Printf("[%s] Request: filtered %d events -> %d events (removed %d)", 
		r.RemoteAddr, originalCount, filteredCount, originalCount-filteredCount)

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Write(filteredData)
}

// handleHealth provides a health check endpoint
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	// Check that CALENDAR_URL is set
	calendarURL, err := getCalendarURL()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}
	log.Printf("Using calendar URL: %s", calendarURL)

	port := defaultPort
	if p := getEnv("PORT", ""); p != "" {
		port = p
	}

	http.HandleFunc("/filter", handleFilter)
	http.HandleFunc("/health", handleHealth)

	log.Printf("Starting calendar filter service on port %s", port)
	log.Printf("Filter endpoint: http://localhost:%s/filter", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

