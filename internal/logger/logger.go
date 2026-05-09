package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Client writes structured JSON logs to stdout and ships them to Loki.
type Client struct {
	pushURL string
	http    *http.Client
	ch      chan entry
	wg      sync.WaitGroup
}

type entry struct {
	ts     time.Time
	level  string
	labels map[string]string
	fields map[string]any
}

func New(lokiURL string) *Client {
	c := &Client{
		pushURL: lokiURL + "/loki/api/v1/push",
		http:    &http.Client{Timeout: 5 * time.Second},
		ch:      make(chan entry, 512),
	}
	c.wg.Add(1)
	go c.run()
	return c
}

func (c *Client) Info(msg string, labels map[string]string, fields map[string]any) {
	c.emit("INFO", msg, labels, fields)
}

func (c *Client) Error(msg string, labels map[string]string, fields map[string]any) {
	c.emit("ERROR", msg, labels, fields)
}

func (c *Client) emit(level, msg string, labels map[string]string, fields map[string]any) {
	// Write structured JSON to stdout immediately.
	out := make(map[string]any, len(fields)+len(labels)+3)
	out["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	out["level"] = level
	out["msg"] = msg
	for k, v := range labels {
		out[k] = v
	}
	for k, v := range fields {
		out[k] = v
	}
	line, err := json.Marshal(out)
	if err == nil {
		fmt.Fprintf(os.Stdout, "%s\n", line)
	}

	// Ship to Loki asynchronously via the background goroutine.
	lokiFields := map[string]any{"msg": msg}
	for k, v := range fields {
		lokiFields[k] = v
	}
	select {
	case c.ch <- entry{ts: time.Now(), level: level, labels: labels, fields: lokiFields}:
	default: // drop if buffer full — never block the request path
	}
}

// Shutdown flushes remaining entries and waits for the goroutine to finish.
func (c *Client) Shutdown() {
	close(c.ch)
	c.wg.Wait()
}

func (c *Client) run() {
	defer c.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var batch []entry

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := c.push(batch); err != nil {
			fmt.Fprintf(os.Stderr, "loki push error: %v\n", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case e, ok := <-c.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, e)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (c *Client) push(batch []entry) error {
	type lokiValues = [][2]string
	type stream struct {
		labels map[string]string
		values lokiValues
	}
	streams := map[string]*stream{}

	for _, e := range batch {
		lbls := make(map[string]string, len(e.labels)+2)
		for k, v := range e.labels {
			lbls[k] = v
		}
		lbls["level"] = e.level
		lbls["service"] = "llm-observatory"

		key := labelsKey(lbls)
		if _, ok := streams[key]; !ok {
			streams[key] = &stream{labels: lbls}
		}

		line, err := json.Marshal(e.fields)
		if err != nil {
			continue
		}
		ts := fmt.Sprintf("%d", e.ts.UnixNano())
		streams[key].values = append(streams[key].values, [2]string{ts, string(line)})
	}

	type lokiStream struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	}
	type lokiBody struct {
		Streams []lokiStream `json:"streams"`
	}

	body := lokiBody{}
	for _, s := range streams {
		body.Streams = append(body.Streams, lokiStream{Stream: s.labels, Values: s.values})
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	resp, err := c.http.Post(c.pushURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	return nil
}

func labelsKey(labels map[string]string) string {
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}
