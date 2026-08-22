package appstream

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var (
	// versionPrefixRegexp matches common tag prefixes like 'v1.0.0', 'V1.0.0', 'release-1.0.0', 'release_1.0.0'
	versionPrefixRegexp = regexp.MustCompile(`^(?:v|V|release[-_])(?:\d)`)
)

// SanitizeVersion cleans a raw git tag or release string into an AppStream-compliant
// version identifier (e.g., 'v1.2.3' -> '1.2.3', 'release-2.0.0' -> '2.0.0').
func SanitizeVersion(tag string) string {
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" {
		return ""
	}
	if versionPrefixRegexp.MatchString(trimmed) {
		if strings.HasPrefix(trimmed, "v") || strings.HasPrefix(trimmed, "V") {
			return trimmed[1:]
		}
		if strings.HasPrefix(trimmed, "release-") {
			return trimmed[len("release-"):]
		}
		if strings.HasPrefix(trimmed, "release_") {
			return trimmed[len("release_"):]
		}
	}
	return trimmed
}

// HasRelease reports whether the metainfo XML already contains a <release> element
// matching the specified version string.
func HasRelease(xmlData []byte, version string) (bool, error) {
	cleanVer := SanitizeVersion(version)
	if cleanVer == "" {
		return false, fmt.Errorf("appstream: empty version provided")
	}

	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, fmt.Errorf("appstream: invalid XML structure: %w", err)
		}

		if se, ok := token.(xml.StartElement); ok && se.Name.Local == "release" {
			for _, attr := range se.Attr {
				if attr.Name.Local == "version" {
					if SanitizeVersion(attr.Value) == cleanVer {
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

// ReleaseOptions holds parameters for injecting or updating a release entry.
type ReleaseOptions struct {
	Version     string
	Date        string
	Description string
	URL         string
	Urgency     string
	Type        string
}

// buildReleaseElement formats the <release> XML node with proper indentation and child elements.
func buildReleaseElement(opts ReleaseOptions, indent string) string {
	var sb strings.Builder
	cleanVer := SanitizeVersion(opts.Version)

	attrs := fmt.Sprintf(`version="%s"`, xmlEscape(cleanVer))
	if opts.Date != "" {
		attrs += fmt.Sprintf(` date="%s"`, xmlEscape(opts.Date))
	}
	if opts.Urgency != "" {
		attrs += fmt.Sprintf(` urgency="%s"`, xmlEscape(opts.Urgency))
	}
	if opts.Type != "" {
		attrs += fmt.Sprintf(` type="%s"`, xmlEscape(opts.Type))
	}

	hasChildren := opts.Description != "" || opts.URL != ""
	if !hasChildren {
		sb.WriteString(fmt.Sprintf("%s<release %s/>", indent, attrs))
		return sb.String()
	}

	childIndent := indent + "  "
	sb.WriteString(fmt.Sprintf("%s<release %s>\n", indent, attrs))
	if opts.Description != "" {
		sb.WriteString(fmt.Sprintf("%s<description>\n", childIndent))
		lines := strings.Split(strings.TrimSpace(opts.Description), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "<p>") || strings.HasPrefix(trimmed, "<ul>") || strings.HasPrefix(trimmed, "<ol>") || strings.HasPrefix(trimmed, "<li>") {
				sb.WriteString(fmt.Sprintf("%s  %s\n", childIndent, trimmed))
			} else {
				sb.WriteString(fmt.Sprintf("%s  <p>%s</p>\n", childIndent, xmlEscape(trimmed)))
			}
		}
		sb.WriteString(fmt.Sprintf("%s</description>\n", childIndent))
	}
	if opts.URL != "" {
		sb.WriteString(fmt.Sprintf("%s<url>%s</url>\n", childIndent, xmlEscape(opts.URL)))
	}
	sb.WriteString(fmt.Sprintf("%s</release>", indent))
	return sb.String()
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// detectLineIndentation inspects the line preceding offset to determine its whitespace prefix.
func detectLineIndentation(content []byte, offset int) string {
	if offset <= 0 || offset > len(content) {
		return ""
	}
	start := offset - 1
	for start >= 0 && content[start] != '\n' && content[start] != '\r' {
		start--
	}
	indent := ""
	for i := start + 1; i < offset && i < len(content); i++ {
		if content[i] == ' ' || content[i] == '\t' {
			indent += string(content[i])
		} else {
			break
		}
	}
	return indent
}

// SyncRelease updates or injects a <release> element into the metainfo XML.
// If the version already exists in the document, it returns the original XML unchanged with modified=false.
// If the XML is updated, it returns the new XML with modified=true.
func SyncRelease(xmlData []byte, opts ReleaseOptions) (updated []byte, modified bool, err error) {
	opts.Version = SanitizeVersion(opts.Version)
	if opts.Version == "" {
		return xmlData, false, fmt.Errorf("appstream: empty release version")
	}

	exists, err := HasRelease(xmlData, opts.Version)
	if err != nil {
		return xmlData, false, err
	}
	if exists {
		return xmlData, false, nil
	}

	// Tokenize XML to locate <releases> or </component>
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	var releasesStartOffset int = -1
	var releasesTagEndOffset int = -1
	var releasesEndOffset int = -1
	var componentEndStartOffset int = -1
	var isSelfClosingReleases bool

	type tokenPos struct {
		name        string
		isStart     bool
		isEnd       bool
		startOffset int
		endOffset   int
	}

	var tokens []tokenPos

	for {
		startOffset := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xmlData, false, fmt.Errorf("appstream: invalid XML structure: %w", err)
		}
		endOffset := int(decoder.InputOffset())

		switch t := token.(type) {
		case xml.StartElement:
			tokens = append(tokens, tokenPos{name: t.Name.Local, isStart: true, startOffset: startOffset, endOffset: endOffset})
		case xml.EndElement:
			tokens = append(tokens, tokenPos{name: t.Name.Local, isEnd: true, startOffset: startOffset, endOffset: endOffset})
		}
	}

	// Locate <releases> start or </component> / </application>
	for i, t := range tokens {
		if t.isStart && t.name == "releases" {
			releasesStartOffset = t.startOffset
			releasesTagEndOffset = t.endOffset
			// Check if immediately followed by EndElement and contains only whitespace (self-closing <releases/> or empty <releases></releases>)
			if i+1 < len(tokens) && tokens[i+1].isEnd && tokens[i+1].name == "releases" {
				if t.endOffset <= tokens[i+1].startOffset {
					rawBetween := xmlData[t.endOffset:tokens[i+1].startOffset]
					if len(bytes.TrimSpace(rawBetween)) == 0 {
						releasesEndOffset = tokens[i+1].endOffset
						isSelfClosingReleases = true
					}
				}
			}
			break
		}
		if t.isEnd && (t.name == "component" || t.name == "application") {
			componentEndStartOffset = t.startOffset
		}
	}

	// Derive child indentation and indentation unit from existing child elements
	var childIndent string
	for _, t := range tokens {
		if t.isStart && t.name != "component" && t.name != "application" {
			ind := detectLineIndentation(xmlData, t.startOffset)
			if ind != "" {
				childIndent = ind
				break
			}
		}
	}
	if childIndent == "" {
		childIndent = "  "
	}
	indentUnit := "  "
	if strings.Contains(childIndent, "\t") {
		indentUnit = "\t"
	} else if len(childIndent) > 0 {
		indentUnit = childIndent
	}

	// Scenario 1: <releases> element found
	if releasesStartOffset != -1 {
		relIndent := detectLineIndentation(xmlData, releasesStartOffset)
		if relIndent == "" {
			relIndent = childIndent
		}

		if isSelfClosingReleases && releasesEndOffset != -1 {
			relNode := buildReleaseElement(opts, relIndent+indentUnit)
			newBlock := fmt.Sprintf("<releases>\n%s\n%s</releases>", relNode, relIndent)
			var buf bytes.Buffer
			buf.Write(xmlData[:releasesStartOffset])
			buf.WriteString(newBlock)
			buf.Write(xmlData[releasesEndOffset:])
			return buf.Bytes(), true, nil
		}

		insertionPoint := releasesTagEndOffset
		if insertionPoint == -1 {
			return xmlData, false, fmt.Errorf("appstream: malformed <releases> start tag")
		}

		relNode := buildReleaseElement(opts, relIndent+indentUnit)

		var buf bytes.Buffer
		buf.Write(xmlData[:insertionPoint])
		buf.WriteString("\n" + relNode)
		buf.Write(xmlData[insertionPoint:])
		return buf.Bytes(), true, nil
	}

	// Scenario 2: No <releases> element, insert before </component> (or </application>)
	if componentEndStartOffset != -1 {
		relNode := buildReleaseElement(opts, childIndent+indentUnit)
		newReleasesBlock := fmt.Sprintf("%s<releases>\n%s\n%s</releases>\n", childIndent, relNode, childIndent)

		var buf bytes.Buffer
		buf.Write(xmlData[:componentEndStartOffset])
		buf.WriteString(newReleasesBlock)
		buf.Write(xmlData[componentEndStartOffset:])
		return buf.Bytes(), true, nil
	}

	return xmlData, false, fmt.Errorf("appstream: neither <releases> nor </component> root element found in XML")
}
