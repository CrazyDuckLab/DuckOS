// Package api collection envelope (G06): every large
// list returned by the API carries a snapshot id, an
// optional cursor, a total (or null when the source is
// unknown), a `complete` flag, and a slice of issues. The
// cursor is opaque to the caller; the snapshot id is a
// server-side version stamp that the caller can echo back
// on the next request for diff-style pagination.
package api

// Collection is the public envelope for every paginated
// read. Items is the loaded page; Total is the size of the
// underlying set when known, or null when the source
// cannot compute it (live captures, search results across
// many stores). Complete is false when the caller may have
// received a partial view (the cursor points at the next
// chunk).
type Collection struct {
	SnapshotID string            `json:"snapshotId"`
	Cursor     string            `json:"cursor,omitempty"`
	Total      *int              `json:"total"`
	Complete   bool              `json:"complete"`
	Issues     []CollectionIssue `json:"issues,omitempty"`
	Items      any               `json:"items"`
}

// CollectionIssue is a per-source diagnostic that the UI
// may surface as a non-fatal warning row. ReasonId is a
// stable code; Source identifies the producer (engine,
// agent, store name).
type CollectionIssue struct {
	Source   string `json:"source"`
	ReasonID string `json:"reasonId"`
}

// CollectionParams is the parsed representation of the
// pagination query string. Default limit is 256; max limit
// is 4096. An empty After means "first page".
type CollectionParams struct {
	Limit int    `json:"limit"`
	After string `json:"after,omitempty"`
}

// DefaultCollectionLimit is the page size used when the
// caller did not specify ?limit=. The cap protects the API
// from accidentally loading the entire firewall table or
// route table into one response.
const DefaultCollectionLimit = 256

// MaxCollectionLimit is the largest page a single request
// can return. Larger windows must be iterated with the
// cursor.
const MaxCollectionLimit = 4096

// ParseCollectionParams extracts limit / after from a URL
// query string. Unrecognised keys are ignored so the
// caller cannot smuggle additional parameters.
func ParseCollectionParams(rawQuery string) CollectionParams {
	p := CollectionParams{Limit: DefaultCollectionLimit}
	if rawQuery == "" {
		return p
	}
	for _, kv := range splitQuery(rawQuery) {
		switch kv.key {
		case "limit":
			n, err := parsePositiveInt(kv.value)
			if err == nil {
				p.Limit = n
			}
		case "after", "cursor":
			p.After = kv.value
		}
	}
	if p.Limit <= 0 {
		p.Limit = DefaultCollectionLimit
	}
	if p.Limit > MaxCollectionLimit {
		p.Limit = MaxCollectionLimit
	}
	return p
}

type queryKV struct{ key, value string }

func splitQuery(raw string) []queryKV {
	out := []queryKV{}
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == '&' {
			seg := raw[start:i]
			start = i + 1
			if seg == "" {
				continue
			}
			eq := -1
			for j := 0; j < len(seg); j++ {
				if seg[j] == '=' {
					eq = j
					break
				}
			}
			if eq < 0 {
				out = append(out, queryKV{key: seg, value: ""})
			} else {
				out = append(out, queryKV{key: seg[:eq], value: seg[eq+1:]})
			}
		}
	}
	return out
}

func parsePositiveInt(s string) (int, error) {
	if s == "" {
		return 0, errBadInt
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errBadInt
		}
		n = n*10 + int(c-'0')
		if n > MaxCollectionLimit {
			return MaxCollectionLimit, nil
		}
	}
	return n, nil
}

var errBadInt = &collectionError{}

type collectionError struct{}

func (*collectionError) Error() string { return "invalid pagination integer" }
