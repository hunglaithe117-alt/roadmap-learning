package roadmap

import (
	"fmt"
	"strings"
)

// BookmarkStatus represents the status of an independent bookmark in roadmap_bookmarks.
type BookmarkStatus string

const (
	// BookmarkToRead indicates an unread bookmark.
	BookmarkToRead BookmarkStatus = "to_read"
	// BookmarkReading indicates a bookmark currently being read.
	BookmarkReading BookmarkStatus = "reading"
	// BookmarkDone indicates a completed bookmark.
	BookmarkDone BookmarkStatus = "done"
	// BookmarkArchived indicates an archived bookmark.
	BookmarkArchived BookmarkStatus = "archived"
)

// BookmarkStatusDefault is the default status for new bookmarks.
const BookmarkStatusDefault = BookmarkToRead

// AllBookmarkStatuses lists all valid bookmark statuses.
var AllBookmarkStatuses = []BookmarkStatus{
	BookmarkToRead, BookmarkReading, BookmarkDone, BookmarkArchived,
}

// Valid reports whether s is a recognized BookmarkStatus.
func (s BookmarkStatus) Valid() bool {
	for _, v := range AllBookmarkStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// ParseBookmarkStatus parses raw status input, defaulting to BookmarkToRead if empty.
func ParseBookmarkStatus(raw string) (BookmarkStatus, error) {
	s := BookmarkStatus(strings.ToLower(strings.TrimSpace(raw)))
	if s == "" {
		return BookmarkStatusDefault, nil
	}
	if !s.Valid() {
		names := make([]string, 0, len(AllBookmarkStatuses))
		for _, v := range AllBookmarkStatuses {
			names = append(names, string(v))
		}
		return "", fmt.Errorf("status bookmark chỉ nhận: %s", strings.Join(names, ", "))
	}
	return s, nil
}

// Tag constraint limits.
const (
	MaxBookmarkTags     = 10
	MaxBookmarkTagRunes = 40
)

// NormalizeTags trims, lowercases, removes blanks and duplicates while preserving insertion order.
func NormalizeTags(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		t := strings.ToLower(strings.TrimSpace(raw))
		if t == "" {
			continue
		}
		if strings.Contains(t, ",") {
			return nil, fmt.Errorf("tag không được chứa dấu phẩy")
		}
		if len([]rune(t)) > MaxBookmarkTagRunes {
			return nil, fmt.Errorf("tag dài tối đa %d ký tự", MaxBookmarkTagRunes)
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > MaxBookmarkTags {
		return nil, fmt.Errorf("tối đa %d tag mỗi bookmark", MaxBookmarkTags)
	}
	return out, nil
}

// EncodeTags serializes tags to a comma-separated string.
func EncodeTags(tags []string) string { return strings.Join(tags, ",") }

// DecodeTags deserializes a comma-separated string into a tag slice.
func DecodeTags(csv string) []string {
	out := []string{}
	for _, part := range strings.Split(csv, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// NormalizeTagFilter normalizes a single filter tag.
func NormalizeTagFilter(raw string) (string, error) {
	t, err := NormalizeTags([]string{raw})
	if err != nil {
		return "", err
	}
	if len(t) == 0 {
		return "", nil
	}
	return t[0], nil
}

// TagsContain reports whether a CSV tag string contains the specified tag.
func TagsContain(csv, tag string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return false
	}
	for _, part := range strings.Split(csv, ",") {
		if strings.ToLower(strings.TrimSpace(part)) == tag {
			return true
		}
	}
	return false
}
