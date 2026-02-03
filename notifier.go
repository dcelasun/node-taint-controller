package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/containrrr/shoutrrr"
	"github.com/containrrr/shoutrrr/pkg/router"
	"github.com/containrrr/shoutrrr/pkg/types"
)

type Notifier struct {
	router *router.ServiceRouter
}

func NewNotifierFromEnv() *Notifier {
	urlsRaw := os.Getenv("SHOUTRRR_URLS")
	if urlsRaw == "" {
		return nil
	}

	urls := strings.Split(urlsRaw, ",")
	var validURLs []string
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u != "" {
			validURLs = append(validURLs, u)
		}
	}

	if len(validURLs) == 0 {
		return nil
	}

	r, err := shoutrrr.CreateSender(validURLs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create shoutrrr sender: %v\n", err)
		return nil
	}

	return &Notifier{router: r}
}

func (n *Notifier) Send(message string) {
	if n == nil || n.router == nil {
		return
	}

	go func() {
		_ = n.router.Send(message, &types.Params{})
	}()
}

func (n *Notifier) Sendf(format string, args ...any) {
	n.Send(fmt.Sprintf(format, args...))
}

func (n *Notifier) TaintAdded(nodeName string) {
	n.Sendf("🚫 Node %s marked out-of-service (NotReady threshold exceeded)", nodeName)
}

func (n *Notifier) TaintRemoved(nodeName string) {
	n.Sendf("✅ Node %s back in service (now Ready)", nodeName)
}

func (n *Notifier) Error(context string, err error) {
	n.Sendf("❌ Error [%s]: %v", context, err)
}
