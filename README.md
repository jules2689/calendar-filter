# Calendar Filter Service

A Golang web service that proxies a Google Calendar iCal feed and filters out events based on recurring time blocks. This is useful for filtering out known "focus" blocks or other recurring time periods from your calendar.

## Features

- Fetches calendar data from Google Calendar iCal feed
- Filters events based on recurring time ranges (e.g., 09:00-10:00 daily)
- Supports multiple filter ranges
- Returns filtered iCal format that can be subscribed to by calendar applications

## Usage

### Starting the Service

```bash
go mod download
go run main.go
```

The service will start on port 8080 by default. You can change this by setting the `PORT` environment variable:

```bash
PORT=3000 go run main.go
```

### Filtering via Query Parameters

Filter events using query parameters. You can specify ranges in two ways:

**Option 1: Simple comma-separated list (recommended)**
```bash
# Filter out events between 9:00 AM and 10:00 AM daily
curl "http://localhost:8080/filter?ranges=09:00-10:00"

# Filter out multiple time blocks
curl "http://localhost:8080/filter?ranges=09:00-10:00,14:00-15:00"
```

**Option 2: Repeating start/end pairs**
```bash
# Filter out events between 9:00 AM and 10:00 AM daily
curl "http://localhost:8080/filter?start=09:00&end=10:00"

# Filter out multiple time blocks
curl "http://localhost:8080/filter?start=09:00&end=10:00&start=14:00&end=15:00"
```

### Filtering via JSON POST

You can also send a POST request with JSON body:

```bash
curl -X POST http://localhost:8080/filter \
  -H "Content-Type: application/json" \
  -d '{
    "time_ranges": [
      {"start": "2024-01-01T09:00:00Z", "end": "2024-01-01T10:00:00Z"},
      {"start": "2024-01-01T14:00:00Z", "end": "2024-01-01T15:00:00Z"}
    ]
  }'
```

Note: When using JSON, the time components (hour and minute) from the provided timestamps are used as daily recurring blocks.

### Health Check

Check if the service is running:

```bash
curl http://localhost:8080/health
```

## How It Works

1. The service fetches the iCal feed from the configured Google Calendar URL
2. It parses the calendar events
3. For each event, it checks if it overlaps with any of the specified filter time ranges
4. Events that overlap with filter ranges are removed
5. The filtered calendar is returned in iCal format

## Filter Logic

- Filter ranges are treated as **daily recurring blocks**. For example, specifying `09:00-10:00` will filter out any events that overlap with 9-10 AM on any day.
- Events are filtered out if they overlap with **any** of the specified time ranges.
- The overlap check considers events that span multiple days.

## Configuration

### Environment Variables

- `CALENDAR_URL`: **Required** - The iCal URL to proxy
- `PORT`: The port to run the server on (defaults to 8080)

Example:
```bash
export CALENDAR_URL="https://calendar.google.com/calendar/ical/YOUR_EMAIL/public/basic.ics"
export PORT=3000
go run main.go
```

**Note:** The service will fail to start if `CALENDAR_URL` is not set.

### Docker

Build and run with Docker:

```bash
# Build the image
docker build -t cal-filter .

# Run with required calendar URL
docker run -p 8080:8080 -e CALENDAR_URL="https://calendar.google.com/calendar/ical/YOUR_EMAIL/public/basic.ics" cal-filter

# Run with custom port
docker run -p 3000:3000 -e PORT=3000 cal-filter
```

## Example Use Case

If you have "Busy" blocks in your calendar but want to filter out known focus time blocks (e.g., 9-10 AM and 2-3 PM daily), you can:

1. Start the service
2. Access the filtered calendar at: `http://localhost:8080/filter?start=09:00&end=10:00&start=14:00&end=15:00`
3. Subscribe to this URL in your calendar application to see your calendar with focus blocks filtered out

## Building

```bash
go build -o cal-filter
./cal-filter
```

