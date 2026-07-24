package tui

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

func getCodeBlockStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(width - 8).
		Background(lipgloss.Color("#1e1e2e")).
		PaddingLeft(2).
		PaddingRight(2).
		PaddingTop(1).
		PaddingBottom(1)
}

func RenderMarkdown(text string, width int) string {
	lines := strings.Split(text, "\n")
	var resultLines []string

	inTable := false
	var tableLines []string

	inCodeBlock := false
	var codeBlockLines []string
	currentLang := ""

	wrapStyle := lipgloss.NewStyle().Width(width)

	flushTable := func() {
		if len(tableLines) == 0 {
			return
		}
		renderedTable := renderPrettyTable(tableLines, width)
		resultLines = append(resultLines, renderedTable...)
		tableLines = nil
		inTable = false
	}

	flushCodeBlock := func() {
		if len(codeBlockLines) == 0 {
			return
		}
		fullCode := strings.Join(codeBlockLines, "\n")
		highlighted := highlightCodeBlock(fullCode, currentLang, width-12)
		
		codeContent := strings.Join(highlighted, "\n")
		codeStyle := getCodeBlockStyle(width)
		styled := codeStyle.Render(codeContent)
		resultLines = append(resultLines, strings.Split(styled, "\n")...)
		
		codeBlockLines = nil
		currentLang = ""
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if inTable {
				flushTable()
			}
			if inCodeBlock {
				flushCodeBlock()
				inCodeBlock = false
			} else {
				inCodeBlock = true
				currentLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			}
			continue
		}

		if inCodeBlock {
			codeBlockLines = append(codeBlockLines, line)
			continue
		}

		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			inTable = true
			tableLines = append(tableLines, trimmed)
		} else {
			if inTable {
				flushTable()
			}
			if trimmed == "" {
				resultLines = append(resultLines, "")
			} else {
				formatted := formatInlineCode(formatItalic(formatBold(line)))
				wrapped := wrapStyle.Render(formatted)
				resultLines = append(resultLines, strings.Split(wrapped, "\n")...)
			}
		}
	}
	if inTable {
		flushTable()
	}
	if inCodeBlock {
		flushCodeBlock()
	}

	return strings.Join(resultLines, "\n")
}

func highlightCodeBlock(code string, lang string, width int) []string {
	if lang == "" {
		lang = "text"
	}

	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal16m")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return indentCodeBlock(code, width)
	}

	var buf bytes.Buffer
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return indentCodeBlock(code, width)
	}

	highlighted := buf.String()
	resultLines := strings.Split(highlighted, "\n")

	if len(resultLines) > 0 && resultLines[len(resultLines)-1] == "" {
		resultLines = resultLines[:len(resultLines)-1]
	}

	var indented []string
	for _, line := range resultLines {
		indented = append(indented, "    "+line)
	}

	return indented
}

func indentCodeBlock(code string, width int) []string {
	lines := strings.Split(code, "\n")
	var result []string
	for _, line := range lines {
		wrapped := lipgloss.NewStyle().Width(width - 4).Render(line)
		wrappedLines := strings.Split(wrapped, "\n")
		for _, wl := range wrappedLines {
			result = append(result, "    "+wl)
		}
	}
	return result
}

func formatBold(text string) string {
	var builder strings.Builder
	parts := strings.Split(text, "**")
	for i, part := range parts {
		if i%2 == 1 {
			builder.WriteString(StyleModelLabel.Render(part))
		} else {
			builder.WriteString(part)
		}
	}
	return builder.String()
}

func formatItalic(text string) string {
	var builder strings.Builder
	parts := strings.Split(text, "*")
	for i, part := range parts {
		if i%2 == 1 {
			builder.WriteString(StyleItalic.Render(part))
		} else {
			builder.WriteString(part)
		}
	}
	return builder.String()
}

func formatInlineCode(text string) string {
	var builder strings.Builder
	parts := strings.Split(text, "`")
	for i, part := range parts {
		if i%2 == 1 {
			builder.WriteString(StyleShortcut.Render(part))
		} else {
			builder.WriteString(part)
		}
	}
	return builder.String()
}

func renderPrettyTable(rawLines []string, termWidth int) []string {
	if len(rawLines) == 0 {
		return nil
	}

	var rows [][]string
	for _, rawLine := range rawLines {
		trimmed := strings.Trim(rawLine, "|")
		if strings.Contains(trimmed, "---") || strings.Contains(trimmed, "===") {
			continue
		}
		parts := strings.Split(rawLine, "|")
		if len(parts) > 2 {
			parts = parts[1 : len(parts)-1]
		}
		var row []string
		for _, part := range parts {
			row = append(row, strings.TrimSpace(part))
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil
	}

	numCols := 0
	for _, r := range rows {
		if len(r) > numCols {
			numCols = len(r)
		}
	}

	colWidths := make([]int, numCols)
	for _, r := range rows {
		for i, val := range r {
			if i < len(colWidths) && len(val) > colWidths[i] {
				colWidths[i] = len(val)
			}
		}
	}

	borderOverhead := 3*numCols + 6

	totalWidth := 0
	for _, w := range colWidths {
		totalWidth += w
	}

	if totalWidth+borderOverhead > termWidth && numCols > 1 {
		lastColIdx := numCols - 1
		otherWidths := 0
		for i := 0; i < lastColIdx; i++ {
			otherWidths += colWidths[i]
		}

		maxLastColWidth := termWidth - otherWidths - borderOverhead
		if maxLastColWidth < 20 {
			maxLastColWidth = 20
		}
		colWidths[lastColIdx] = maxLastColWidth
	}

	var rendered []string

	drawBorderLine := func(left, middle, right, dash string) string {
		var parts []string
		for _, w := range colWidths {
			parts = append(parts, strings.Repeat(dash, w+2))
		}
		return "  " + left + strings.Join(parts, middle) + right
	}

	rendered = append(rendered, drawBorderLine("┌", "┬", "┐", "─"))

	for idx, row := range rows {
		cellLines := make([][]string, numCols)
		maxLines := 1

		for i := 0; i < numCols; i++ {
			val := ""
			if i < len(row) {
				val = row[i]
			}

			lines := wrapText(val, colWidths[i])
			cellLines[i] = lines
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}

		for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
			renderedLine := "  │ "
			for i := 0; i < numCols; i++ {
				val := ""
				if lineIdx < len(cellLines[i]) {
					val = cellLines[i][lineIdx]
				}
				padWidth := colWidths[i] - len(val)
				if padWidth < 0 {
					padWidth = 0
				}
				padded := val + strings.Repeat(" ", padWidth)

				if idx == 0 {
					renderedLine += StyleModelLabel.Render(padded) + " │ "
				} else {
					renderedLine += formatBold(padded) + " │ "
				}
			}
			renderedLine = strings.TrimSuffix(renderedLine, " ")
			rendered = append(rendered, renderedLine)
		}

		if idx == 0 && len(rows) > 1 {
			rendered = append(rendered, drawBorderLine("├", "┼", "┤", "─"))
		}
	}

	rendered = append(rendered, drawBorderLine("└", "┴", "┘", "─"))

	return rendered
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if len(text) <= width {
		return []string{text}
	}
	var lines []string
	for len(text) > width {
		wrapIdx := width
		spaceIdx := strings.LastIndex(text[:width], " ")
		if spaceIdx > 0 {
			wrapIdx = spaceIdx
		}
		lines = append(lines, text[:wrapIdx])
		text = strings.TrimSpace(text[wrapIdx:])
	}
	if len(text) > 0 {
		lines = append(lines, text)
	}
	return lines
}
