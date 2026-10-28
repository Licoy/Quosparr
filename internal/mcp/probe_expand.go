package mcp

import (
	"net/url"
	"os"
	"sort"
	"strings"
)

func probeExpandDraft(d Draft) (Draft, []string, error) {
	secrets := []string{}
	expand := func(value string) (string, error) {
		var b strings.Builder
		for {
			start := strings.Index(value, "${")
			if start < 0 {
				b.WriteString(value)
				break
			}
			b.WriteString(value[:start])
			value = value[start+2:]
			end := strings.IndexByte(value, '}')
			if end < 0 {
				return "", probeErr("invalid_draft", "connect")
			}
			key := value[:end]
			if !envName.MatchString(key) {
				return "", probeErr("invalid_draft", "connect")
			}
			v, ok := os.LookupEnv(key)
			if !ok || v == "" {
				return "", probeErr("missing_env", "connect")
			}
			secrets = append(secrets, v)
			b.WriteString(v)
			value = value[end+1:]
		}
		return b.String(), nil
	}
	var err error
	for _, target := range []*string{&d.Command, &d.Cwd} {
		*target, err = expand(*target)
		if err != nil {
			return d, nil, err
		}
	}
	args := make([]string, len(d.Args))
	for i, v := range d.Args {
		args[i], err = expand(v)
		if err != nil {
			return d, nil, err
		}
		if args[i] != "" {
			secrets = append(secrets, args[i])
		}
	}
	d.Args = args
	for _, target := range []*map[string]string{&d.Env, &d.Headers} {
		values := map[string]string{}
		for k, v := range *target {
			value, e := expand(v)
			if e != nil {
				return d, nil, e
			}
			values[k] = value
			secrets = append(secrets, value)
			if strings.EqualFold(k, "authorization") {
				_, token, _ := strings.Cut(value, " ")
				secrets = append(secrets, token)
			}
		}
		*target = values
	}
	if u, e := url.Parse(d.URL); e == nil {
		for _, values := range u.Query() {
			secrets = append(secrets, values...)
		}
		for _, part := range strings.Split(u.Path, "/") {
			if part != "" {
				secrets = append(secrets, part)
			}
		}
	}
	secrets = append(secrets, d.Command, d.Cwd, d.URL)
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return d, secrets, nil
}
