package handlers

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// archiveHTTPClient is used to fetch attachment bytes before a message is
// deleted. Discord CDN URLs stop working shortly after deletion, so callers
// must download while the message still exists.
var archiveHTTPClient = &http.Client{Timeout: 20 * time.Second}

// maxArchiveBytes caps a single archived attachment. Discord's own upload limit
// for most guilds is 25 MiB; we stay under that so re-uploads succeed.
const maxArchiveBytes = 8 << 20 // 8 MiB

// archivedAttachment holds a downloaded attachment ready to be re-uploaded.
type archivedAttachment struct {
	Name        string
	ContentType string
	Data        []byte
}

// downloadAttachments fetches the bytes of every attachment on m so they can be
// preserved in a log message even after the original is deleted. Attachments
// larger than maxArchiveBytes are skipped (their URL is still logged elsewhere).
// The download happens synchronously and must be called BEFORE deleting m.
func downloadAttachments(m *discordgo.Message) []archivedAttachment {
	if m == nil || len(m.Attachments) == 0 {
		return nil
	}
	out := make([]archivedAttachment, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		if a == nil || a.URL == "" {
			continue
		}
		if a.Size > maxArchiveBytes {
			continue
		}
		data, ct := fetchBytes(a.URL, maxArchiveBytes)
		if len(data) == 0 {
			continue
		}
		if ct == "" {
			ct = a.ContentType
		}
		out = append(out, archivedAttachment{
			Name:        safeAttachmentName(a.Filename),
			ContentType: ct,
			Data:        data,
		})
	}
	return out
}

// fetchBytes downloads up to limit bytes from url, returning the body and the
// response Content-Type. It returns nil on any error or if the body exceeds the
// limit.
func fetchBytes(url string, limit int64) ([]byte, string) {
	resp, err := archiveHTTPClient.Get(url)
	if err != nil {
		return nil, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, ""
	}
	return data, resp.Header.Get("Content-Type")
}

// safeAttachmentName ensures a non-empty, path-free filename for re-upload.
func safeAttachmentName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i != -1 {
		name = name[i+1:]
	}
	if name == "" {
		return "attachment"
	}
	return name
}

// asDiscordFiles converts archived attachments into discordgo file uploads.
func asDiscordFiles(atts []archivedAttachment) []*discordgo.File {
	if len(atts) == 0 {
		return nil
	}
	files := make([]*discordgo.File, 0, len(atts))
	for _, a := range atts {
		files = append(files, &discordgo.File{
			Name:        a.Name,
			ContentType: a.ContentType,
			Reader:      bytes.NewReader(a.Data),
		})
	}
	return files
}
