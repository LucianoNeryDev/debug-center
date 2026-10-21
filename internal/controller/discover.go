// Package controller is the technician side: it finds agents on the tailnet
// through the local tailscaled and talks to their control API.
package controller

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"

	"tailscale.com/client/local"

	"git.lucianonery.dev/LucianoNeryDev/debug-center/internal/proto"
)

// AgentTag lets agents be found by tag as well as by hostname prefix.
const AgentTag = "tag:dbc-agent"

// Peer is a tailnet node that looks like an agent.
type Peer struct {
	Hostname string     `json:"hostname"`
	DNSName  string     `json:"dns_name"`
	IP       netip.Addr `json:"ip"`
	Online   bool       `json:"online"`
	LastSeen time.Time  `json:"last_seen,omitempty"`
	OS       string     `json:"os"`
	Tags     []string   `json:"tags,omitempty"`
}

// Discoverer lists agent peers from the local tailscaled.
type Discoverer struct {
	LC *local.Client
}

func (d Discoverer) Peers(ctx context.Context) ([]Peer, error) {
	st, err := d.LC.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("query local tailscaled (is it running and are you its operator? try `sudo tailscale set --operator=$USER`): %w", err)
	}
	var out []Peer
	for _, ps := range st.Peer {
		var tags []string
		if ps.Tags != nil {
			tags = ps.Tags.AsSlice()
		}
		if !strings.HasPrefix(ps.HostName, proto.HostnamePrefix) && !slices.Contains(tags, AgentTag) {
			continue
		}
		p := Peer{
			Hostname: ps.HostName, DNSName: strings.TrimSuffix(ps.DNSName, "."),
			Online: ps.Online, LastSeen: ps.LastSeen, OS: ps.OS, Tags: tags,
		}
		for _, ip := range ps.TailscaleIPs {
			if ip.Is4() || !p.IP.IsValid() {
				p.IP = ip
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		return out[i].Hostname < out[j].Hostname
	})
	return out, nil
}

// Resolve finds the single agent matching query: a tailnet IP, a hostname
// (with or without the dbc- prefix), an agent ID prefix or a unique
// substring of the hostname.
func (d Discoverer) Resolve(ctx context.Context, query string) (Peer, error) {
	if ip, err := netip.ParseAddr(query); err == nil {
		return Peer{Hostname: query, IP: ip, Online: true}, nil
	}
	peers, err := d.Peers(ctx)
	if err != nil {
		return Peer{}, err
	}
	q := strings.ToLower(query)
	var exact, partial []Peer
	for _, p := range peers {
		h := strings.ToLower(p.Hostname)
		short := strings.TrimPrefix(h, proto.HostnamePrefix)
		first, _, _ := strings.Cut(strings.ToLower(p.DNSName), ".")
		switch {
		case h == q, short == q, first == q, p.IP.String() == q,
			len(q) >= 6 && strings.HasSuffix(h, "-"+q[:6]):
			exact = append(exact, p)
		case strings.Contains(h, q):
			partial = append(partial, p)
		}
	}
	pick := func(c []Peer) (Peer, bool, error) {
		switch len(c) {
		case 0:
			return Peer{}, false, nil
		case 1:
			return c[0], true, nil
		}
		var online []Peer
		for _, p := range c {
			if p.Online {
				online = append(online, p)
			}
		}
		if len(online) == 1 {
			return online[0], true, nil
		}
		names := make([]string, len(c))
		for i, p := range c {
			names[i] = p.Hostname
		}
		return Peer{}, false, fmt.Errorf("%q matches several agents: %s", query, strings.Join(names, ", "))
	}
	for _, c := range [][]Peer{exact, partial} {
		if p, ok, err := pick(c); ok || err != nil {
			return p, err
		}
	}
	return Peer{}, fmt.Errorf("no agent matches %q (see `dbc ls --all`)", query)
}
