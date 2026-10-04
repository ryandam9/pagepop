package builder

import (
	"fmt"
	"regexp"
	"strings"
)

// reTweetPara matches a paragraph that holds nothing but a link to a single
// post on X/Twitter — what goldmark produces for a bare URL on its own line
// (GFM autolinks it), for <https://...>, and for [text](https://...). Links
// that sit inside a sentence are left alone.
var reTweetPara = regexp.MustCompile(`<p>\s*<a href="https?://(?:www\.|mobile\.)?(?:x|twitter)\.com/(\w+)/status(?:es)?/(\d+)[^"]*">[^<]*</a>\s*</p>`)

// embedTweets replaces each stand-alone X/Twitter post link with the markup
// that platform.twitter.com/widgets.js upgrades into an embedded post. Without
// JavaScript the blockquote still shows a working link to the post.
func embedTweets(html string) string {
	return reTweetPara.ReplaceAllStringFunc(html, func(match string) string {
		parts := reTweetPara.FindStringSubmatch(match)
		user, id := parts[1], parts[2]
		url := fmt.Sprintf("https://twitter.com/%s/status/%s", user, id)
		label := "View this post on X"
		if user != "i" {
			label = fmt.Sprintf("View @%s's post on X", user)
		}
		return fmt.Sprintf(`<blockquote class="twitter-tweet" data-dnt="true"><p><a href="%s">%s</a></p></blockquote>`, url, label)
	})
}

// hasTweets reports whether a rendered body contains an embedded post, so the
// widget script is only loaded on pages that need it.
func hasTweets(html string) bool {
	return strings.Contains(html, `class="twitter-tweet"`)
}
