package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ocrHTTPClient talks to the ocr.space API.
var ocrHTTPClient = &http.Client{Timeout: 25 * time.Second}

// maxOCRImages caps how many image attachments are scanned per message so a
// flood of images can't hammer the OCR quota.
const maxOCRImages = 3

// ocrSpaceResponse is the subset of the ocr.space JSON response we use.
type ocrSpaceResponse struct {
	ParsedResults []struct {
		ParsedText string `json:"ParsedText"`
	} `json:"ParsedResults"`
	IsErroredOnProcessing bool        `json:"IsErroredOnProcessing"`
	ErrorMessage          interface{} `json:"ErrorMessage"`
}

// scanImagesForScam extracts text from image attachments via OCR and folds any
// scam signals it finds into res. It is a no-op unless OCR is enabled and an
// API key is configured. It must run BEFORE the message is deleted so the
// attachment URLs are still reachable by the OCR service.
func (h *Handler) scanImagesForScam(m *discordgo.Message, res scamResult) scamResult {
	cfg := h.cfg.AntiScam
	if !cfg.OCR.Enabled || cfg.OCR.APIKey == "" || m == nil {
		return res
	}

	scanned := 0
	for _, a := range m.Attachments {
		if a == nil || !isImageAttachment(a) {
			continue
		}
		if scanned >= maxOCRImages {
			break
		}
		scanned++

		text := h.runOCR(a.URL)
		if text == "" {
			continue
		}
		sub := scanForScam(text, m.MentionEveryone, cfg.ExtraDomains, cfg.ExtraKeywords, cfg.BlockLookalikes)
		if sub.score > 0 {
			res.score += sub.score
			for _, r := range sub.reasons {
				res.reasons = append(res.reasons, "image → "+r)
			}
		}
	}
	return res
}

// isImageAttachment reports whether an attachment looks like an image.
func isImageAttachment(a *discordgo.MessageAttachment) bool {
	if strings.HasPrefix(strings.ToLower(a.ContentType), "image/") {
		return true
	}
	name := strings.ToLower(a.Filename)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// runOCR sends an image URL to ocr.space and returns the extracted text.
func (h *Handler) runOCR(imageURL string) string {
	form := url.Values{}
	form.Set("apikey", h.cfg.AntiScam.OCR.APIKey)
	form.Set("url", imageURL)
	form.Set("language", "eng")
	form.Set("scale", "true")

	resp, err := ocrHTTPClient.PostForm("https://api.ocr.space/parse/image", form)
	if err != nil {
		slog.Debug("ocr request failed", "err", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var out ocrSpaceResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		slog.Debug("ocr decode failed", "err", err)
		return ""
	}
	if out.IsErroredOnProcessing || len(out.ParsedResults) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, r := range out.ParsedResults {
		sb.WriteString(r.ParsedText)
		sb.WriteString("\n")
	}
	return sb.String()
}
