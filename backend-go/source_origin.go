package main

import (
	"golang.org/x/net/publicsuffix"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// Conservative provenance groups, never a correctness score. Same publisher
// or near-identical passages must not masquerade as independent corroboration.
// Keep all distinct passages: even a near-copy can contain a changed condition.
func groupSourceOrigins(sources []Source) {
	groups := make([]int, len(sources))
	signatures := make([]map[string]bool, len(sources))
	for i := range sources {
		x := &sources[i]
		x.Origin = ""
		if u, err := url.Parse(x.URL); err == nil && u.Hostname() != "" {
			x.Origin = strings.ToLower(u.Hostname())
			if domain, err := publicsuffix.EffectiveTLDPlusOne(x.Origin); err == nil {
				x.Origin = domain
			}
		} else if x.DocumentID != "" {
			x.Origin = "document:" + x.DocumentID
		}
		groups[i] = i
		signatures[i] = sourcePassageSignature(x.Content)
		for j := 0; j < i; j++ {
			if (x.Origin == "" || x.Origin != sources[j].Origin) && !similarSourcePassage(signatures[i], signatures[j]) {
				continue
			}
			from, to := groups[i], groups[j]
			if from < to {
				from, to = to, from
			}
			for k := 0; k <= i; k++ {
				if groups[k] == from {
					groups[k] = to
				}
			}
		}
	}
	ids := map[int]string{}
	for i := range sources {
		if ids[groups[i]] == "" {
			ids[groups[i]] = "E" + strconv.Itoa(len(ids)+1)
		}
		sources[i].EvidenceGroup = ids[groups[i]]
	}
}

func sourcePassageSignature(body string) map[string]bool {
	r := []rune(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, body))
	out := map[string]bool{}
	if len(r) < 120 {
		return out
	}
	for i := 0; i+8 <= len(r); i++ {
		out[string(r[i:i+8])] = true
	}
	return out
}

func similarSourcePassage(a, b map[string]bool) bool {
	if len(a) < 30 || len(b) < 30 {
		return false
	}
	shared := 0
	for k := range a {
		if b[k] {
			shared++
		}
	}
	return shared*100 >= 85*(len(a)+len(b)-shared)
}
