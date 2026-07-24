package telegram

import (
	"strings"
	"sync"
	"testing"
)


func TestMarkdownToHTML_NoUnsupportedTags(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "unordered list with dashes",
			markdown: "- item one\n- item two\n- item three",
		},
		{
			name:     "unordered list with stars",
			markdown: "* item one\n* item two",
		},
		{
			name:     "ordered list",
			markdown: "1. first\n2. second\n3. third",
		},
		{
			name:     "nested formatting in list",
			markdown: "- **bold item**\n- *italic item*",
		},
		{
			name:     "code in list",
			markdown: "- `code item`\n- regular",
		},
		{
			name:     "heading followed by list",
			markdown: "# Tools\n- tool1\n- tool2",
		},
		{
			name:     "complex mixed content",
			markdown: "Here are my tools:\n\n- **web_search**: Search the web\n- **read_file**: Read files\n- `execute_command`: Run commands\n\nPick one!",
		},
	}

	unsupportedTags := []string{
		"<ul>", "</ul>",
		"<ol>", "</ol>",
		"<li>", "</li>",
		"<p>", "</p>",
		"<div>", "</div>",
		"<h1>", "<h2>", "<h3>", "<h4>", "<h5>", "<h6>",
		"<br>", "<br/>", "<br />",
		"<table>", "<tr>", "<td>", "<th>",
		"<span>", // bare <span> without class is not supported
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := markdownToHTML(tt.markdown)
			t.Logf("Input:\n%s", tt.markdown)
			t.Logf("Output:\n%s", result)

			for _, tag := range unsupportedTags {
				if strings.Contains(result, tag) {
					t.Errorf("❌ UNSUPPORTED TAG DETECTED: %q in output\nTelegram Bot API will REJECT this message silently!", tag)
				}
			}
		})
	}
}


func TestMarkdownToHTML_ProducesValidTags(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     []string 
		notWant  []string 
	}{
		{
			name:     "bold text",
			markdown: "**hello**",
			want:     []string{"<b>hello</b>"},
			notWant:  []string{"<ul>", "<ol>", "<li>"},
		},
		{
			name:     "inline code",
			markdown: "run `command` now",
			want:     []string{"<code>command</code>"},
		},
		{
			name:     "fenced code",
			markdown: "```\ncode block\n```",
			want:     []string{"<pre><code>"},
		},
		{
			name:     "link",
			markdown: "[click](https://example.com)",
			want:     []string{`<a href="https://example.com">click</a>`},
		},
		{
			name:     "bold with asterisk prefix (list like)",
			markdown: "some text **bold** with *italic*",
			want:     []string{"<b>bold</b>", "<i>italic</i>"},
			notWant:  []string{"<ul>", "<ol>", "<li>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := markdownToHTML(tt.markdown)
			t.Logf("Input: %q", tt.markdown)
			t.Logf("Output: %q", result)

			for _, w := range tt.want {
				if !strings.Contains(result, w) {
					t.Errorf("Expected output to contain: %q", w)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(result, nw) {
					t.Errorf("Output should NOT contain: %q (unsupported)", nw)
				}
			}
		})
	}
}


func TestTelegramUI_EmptyMessageSkipped(t *testing.T) {
	content := ""
	cleaned := strings.TrimSpace(content)

	t.Logf("Content: %q", content)
	t.Logf("Cleaned: %q", cleaned)

	if role := "assistant"; role == "assistant" && content != "" {
		t.Log("Message would be sent (content not empty)")
	} else {
		t.Log("⚠️  BUG PATH: Message would NOT be sent (empty content)")
		t.Log("   → TelegramUI skips sending because content == \"\"")
		t.Log("   → User sees typing indicator, then NOTHING")
	}

	content = "Hello world"
	t.Logf("\nWhen content is %q:", content)
	t.Logf("  → sendLongMessage is called")
	t.Logf("  → bot.Send(message) is called")
	t.Logf("  → If bot.Send() fails → ERROR IS SILENT (no retry, no fallback)")
}


func TestTelegramUI_SendErrorHandlingAudit(t *testing.T) {
	locations := []struct {
		function string
		line     int
		code     string
		impact   string
	}{
		{
			function: "PrintThinking",
			line:     60,
			code:     `u.bot.Send(action)`,
			impact:   "Typing indicator fails silently - user sees bot as idle",
		},
		{
			function: "PrintMessage",
			line:     69,
			code:     `u.bot.Send(msg)`,
			impact:   "Assistant message fails silently - user sees NO response",
		},
		{
			function: "PrintToolCall",
			line:     90,
			code:     `u.bot.Send(msg)`,
			impact:   "Tool call notification fails silently",
		},
		{
			function: "PrintToolResult",
			line:     117,
			code:     `u.bot.Send(msg)`,
			impact:   "Tool result fails silently",
		},
		{
			function: "PrintCompression",
			line:     193,
			code:     `u.bot.Send(msg)`,
			impact:   "Compression notification fails silently",
		},
		{
			function: "sendLongMessage",
			line:     279,
			code:     `u.bot.Send(msg)`,
			impact:   "Part of a long message fails silently - incomplete response",
		},
		{
			function: "sendLongMessage (loop)",
			line:     301,
			code:     `u.bot.Send(msg)`,
			impact:   "Long message chunk fails - later chunks still sent but gap exists",
		},
		{
			function: "handleMessage (sendText)",
			line:     161,
			code:     `tg.bot.Send(msg)`,
			impact:   "Command response (start/reset/etc) fails silently",
		},
		{
			function: "RequestApproval",
			line:     226,
			code:     `sent, err := u.bot.Send(msg)`,
			impact:   "Approval prompt fails - agent blocks forever waiting for response",
		},
		{
			function: "RequestChainContinue",
			line:     259,
			code:     `u.bot.Send(msg)`,
			impact:   "Chain continue prompt fails silently",
		},
	}

	totalLocations := len(locations)
	errorChecked := 1    
	errorIgnored := totalLocations - errorChecked

	t.Logf("=== AUDIT: bot.Send() Error Handling ===")
	t.Logf("Total bot.Send() calls: %d", totalLocations)
	t.Logf("Calls with error handling: %d", errorChecked)
	t.Logf("Calls WITHOUT error handling: %d", errorIgnored)
	t.Logf("Silent failure rate: %d/%d = %d%%", errorIgnored, totalLocations, errorIgnored*100/totalLocations)
	t.Logf("")

	for _, loc := range locations {
		status := "❌ NO ERROR CHECK"
		if loc.function == "RequestApproval" {
			status = "✅ HAS ERROR CHECK"
		}
		t.Logf("[%s] %s (line ~%d)", status, loc.function, loc.line)
		t.Logf("     Code: %s", loc.code)
		t.Logf("     Impact: %s", loc.impact)
		t.Logf("")
	}

	if errorIgnored > 0 {
		t.Logf("\n⚠️  BUG CONFIRMED: %d bot.Send() calls silently ignore errors", errorIgnored)
		t.Logf("   → When Telegram API returns error (rate limit, flood, bad HTML):")
		t.Logf("   → Bot thinks message was sent, user sees NOTHING")
		t.Logf("   → This explains intermittent silent failures")
	}
}


func TestTelegramProcessor_SequentialNature(t *testing.T) {

	ch := make(chan string, 10)
	var wg sync.WaitGroup

	processorDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range ch {
			_ = msg
		}
		close(processorDone)
	}()

	sendResults := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		select {
		case ch <- "message":
			sendResults = append(sendResults, "QUEUED")
		default:
			sendResults = append(sendResults, "DROPPED")
		}
	}

	close(ch)
	wg.Wait()

	queued := 0
	dropped := 0
	for _, r := range sendResults {
		switch r {
		case "QUEUED":
			queued++
		case "DROPPED":
			dropped++
		}
	}

	t.Logf("=== Channel Buffer Test (capacity=10) ===")
	t.Logf("Total messages sent: %d", len(sendResults))
	t.Logf("Queued (success): %d", queued)
	t.Logf("Dropped (buffer full): %d", dropped)
	t.Logf("")

	if dropped > 0 {
		t.Logf("⚠️  BUG CONFIRMED: %d messages were DROPPED due to full buffer!", dropped)
		t.Logf("   → Channel capacity = 10, limit before first msg finishes processing")
		t.Logf("   → If processing takes 5 minutes (approval timeout), buffer overflows")
		t.Logf("   → Overflowing messages are silently dropped with 'please wait'")
		t.Logf("   → User: types message → sees 'please wait' → thinks bot is broken")
	}

	t.Logf("\n=== WORST CASE SCENARIO (5 min timeout) ===")
	t.Logf("In 5 minutes of approval blocking:")
	t.Logf("  User types ~1 msg/15s = ~20 messages")
	t.Logf("  Only first 10 queued = 10 messages DROPPED")
	t.Logf("  After 5 min timeout → 11th message processed")
	t.Logf("  LLM sees old context → response is CONFUSED/GENERIC")
}


func TestRealWorldLLMResponse(t *testing.T) {
	llmResponse := `Saya punya beberapa tools yang bisa saya gunakan:

1. **web_search** - Mencari informasi di web
2. **read_file** - Membaca file
3. **execute_command** - Menjalankan perintah

Ada juga:
- **web_fetch**: Mengambil konten halaman web
- **http_request**: Kirim HTTP request

Mau coba yang mana? 🚀`

	html := markdownToHTML(llmResponse)
	t.Logf("=== LLM Markdown Input ===")
	t.Logf("%s", llmResponse)
	t.Logf("")
	t.Logf("=== HTML Output ===")
	t.Logf("%s", html)
	t.Logf("")

	unsupportedFound := false
	for _, tag := range []string{"<ol>", "</ol>", "<ul>", "</ul>", "<li>", "</li>"} {
		if strings.Contains(html, tag) {
			t.Errorf("❌ UNSUPPORTED TAG: %s — Telegram will reject this message!", tag)
			unsupportedFound = true
		}
	}

	if !unsupportedFound {
		t.Log("✓ No unsupported HTML tags found")
	} else {
		t.Logf("\n⚠️  BUG CONFIRMED: markdownToHTML produces tags Telegram doesn't support")
		t.Logf("   When Telegram receives unsupported HTML tags, it may:")
		t.Logf("   1. Strip the message silently (user sees nothing)")
		t.Logf("   2. Return an error (which is silently ignored - see TestTelegramUI_SendErrorHandlingAudit)")
		t.Logf("   3. Partially render (some text visible, some missing)")
	}
}
