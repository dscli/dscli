module github.com/dscli/dscli

go 1.26.3

require (
	github.com/chromedp/chromedp v0.15.1
	github.com/eatmoreapple/openwechat v1.4.0
	github.com/go-ego/gse v1.0.2
	github.com/goccy/go-yaml v1.19.2
	github.com/mattn/go-runewidth v0.0.19
	github.com/spf13/cobra v1.8.0
	golang.org/x/term v0.41.0
	golang.org/x/text v0.35.0
	modernc.org/sqlite v1.50.1
	mvdan.cc/sh/v3 v3.13.1
)

ignore ./docs

require (
	github.com/chromedp/cdproto v0.0.0-20260321001828-e3e3800016bc
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.6.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-json-experiment/json v0.0.0-20260214004413-d219187c3433 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/vcaesar/cedar v0.30.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	modernc.org/libc v1.72.5 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace github.com/go-ego/gse => ./internal/gse
