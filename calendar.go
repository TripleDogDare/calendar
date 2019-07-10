package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	calendar "google.golang.org/api/calendar/v3"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

/*
	MaxResults sets the optional parameter "maxResults": Maximum number of events returned on one result page.
	The number of events in the resulting page may be less than this value, or none at all, even if there are more events matching the query.
	Incomplete pages can be detected by a non-empty nextPageToken field in the response. By default the value is 250 events.
	The page size can never be larger than 2500 events.

	Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
*/
func main() {
	var limit int
	var dateStartString string
	var dateEndString string
	var dateFromSpan time.Duration
	var dateToSpan time.Duration
	var dateStart time.Time
	var dateEnd time.Time
	var err error
	flag.IntVar(&limit, "limit", 250, "Limit number of entries")
	flag.StringVar(&dateStartString, "start", "", "Start date RFC3339 format [2006-01-02T15:04:05Z] (default to now)")
	flag.StringVar(&dateEndString, "end", "", "Start date RFC3339 format [2006-01-02T15:04:05Z] (default to now)")
	flag.DurationVar(&dateFromSpan, "from", dateFromSpan, "Duration to subtract from start date: ")
	flag.DurationVar(&dateToSpan, "to", dateToSpan, "Duration to add to end date")
	flag.Parse()
	ctx := context.Background()

	if dateStartString == "" {
		dateStart = time.Now()
	} else {
		dateStart, err = time.Parse(time.RFC3339, dateStartString)
		if err != nil {
			log.Fatalf("Unable to parse start date: %v", err)
		}
	}
	dateEnd = dateEnd.Add(dateFromSpan)

	if dateEndString == "" {
		dateEnd = time.Now()
	} else {
		dateEnd, err = time.Parse(time.RFC3339, dateEndString)
		if err != nil {
			log.Fatalf("Unable to parse start date: %v", err)
		}
	}
	dateEnd = dateEnd.Add(dateToSpan)

	if !dateEnd.After(dateStart) {
		log.Fatalf("End date must be after start date: %s -> %s", dateStart.Format(time.RFC3339), dateEnd.Format(time.RFC3339))
	}

	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := calendar.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Calendar client: %v", err)
	}

	var collector EventCollector
	fetchEventCtx, fetchEventCancel := context.WithTimeout(ctx, 10*time.Second)
	defer fetchEventCancel()
	err = srv.Events.List("primary").ShowDeleted(false).SingleEvents(true).
		TimeMin(dateStart.Format(time.RFC3339)).TimeMax(dateEnd.Format(time.RFC3339)).
		MaxResults(10).OrderBy("startTime").Pages(fetchEventCtx, collector.WriteCallback(fetchEventCtx, os.Stdout))
	if err != nil {
		log.Fatalf("Unable to retrieve events: %v", err)
	}
}

func WriteEvent(w *csv.Writer, item *calendar.Event) error {
	date := item.Start.DateTime
	if date == "" {
		date = item.Start.Date
	}
	return w.Write([]string{date, item.Summary})

}

type EventCollector struct {
	events      []*calendar.Events
	pageCounter int
	itemCounter int
}

func (c *EventCollector) WriteCallback(ctx context.Context, w io.Writer) func(e *calendar.Events) error {
	csvWriter := csv.NewWriter(w)
	return func(e *calendar.Events) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.pageCounter++
		c.itemCounter += len(e.Items)
		for _, item := range e.Items {
			err := WriteEvent(csvWriter, item)
			if err != nil {
				return err
			}
			csvWriter.Flush()
		}
		return nil
	}
}
