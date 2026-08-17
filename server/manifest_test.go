package main

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const officialMarketplaceIconSHA256 = "3a67c3dcc2a1655d9f3ea1051817c4c8849d802383e45dd6cec1b910ff874922"

func TestManifestIncludesPackagedMarketplaceIcon(t *testing.T) {
	contents, err := os.ReadFile("../manifest.yaml")
	if err != nil {
		t.Fatal(err)
	}

	iconPath := manifestIconPath(string(contents))
	if iconPath != "assets/icon.svg" {
		t.Fatalf("manifest icon = %q, want %q", iconPath, "assets/icon.svg")
	}

	icon, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(iconPath)))
	if err != nil {
		t.Fatal(err)
	}
	assertMarketplaceSVG(t, icon)
}

func manifestIconPath(manifest string) string {
	for _, line := range strings.Split(manifest, "\n") {
		if strings.HasPrefix(line, "icon:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "icon:")), `"`)
		}
	}
	return ""
}

func assertMarketplaceSVG(t *testing.T, icon []byte) {
	t.Helper()
	if got := fmt.Sprintf("%x", sha256.Sum256(icon)); got != officialMarketplaceIconSHA256 {
		t.Fatalf("marketplace SVG sha256 = %q, want official Slack asset %q", got, officialMarketplaceIconSHA256)
	}

	var root struct {
		XMLName xml.Name
		Width   string `xml:"width,attr"`
		Height  string `xml:"height,attr"`
		ViewBox string `xml:"viewBox,attr"`
	}
	if err := xml.Unmarshal(icon, &root); err != nil {
		t.Fatal(err)
	}
	if root.XMLName.Local != "svg" || root.Width != "54" || root.Height != "54" || root.ViewBox != "0 0 54 54" {
		t.Fatalf("unexpected SVG root: name=%q width=%q height=%q viewBox=%q", root.XMLName.Local, root.Width, root.Height, root.ViewBox)
	}

	lower := strings.ToLower(string(icon))
	// The official SVG uses url(#...) for an internal clip path. Its pinned
	// digest prevents that reference from being replaced with external content.
	for _, forbidden := range []string{"<script", "<foreignobject", "href=", "currentcolor"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("marketplace SVG contains forbidden content %q", forbidden)
		}
	}
}
