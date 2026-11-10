package codexdesktop

import "encoding/json"

func mergePartition(dst, src partition) (partition, error) {
	out := partition{clone(dst.Globals), clone(dst.Atoms)}
	if err := checkThreadOwnership(dst.Globals, src.Globals); err != nil {
		return out, err
	}
	if err := checkProjects(dst.Globals, src.Globals); err != nil {
		return out, err
	}
	for key, raw := range src.Globals {
		var merged json.RawMessage
		var err error
		switch key {
		case "app-server-project-id-by-legacy-project-id-by-host":
			merged, err = mergeHosts(raw, dst.Globals[key])
		case "thread-project-assignments", "thread-project-membership-host-ids":
			merged, err = mergeMap(raw, dst.Globals[key], true)
		case "sidebar-project-thread-orders":
			merged, err = mergeOrders(raw, dst.Globals[key])
		default:
			merged, err = mergeValue(raw, dst.Globals[key])
		}
		if err != nil {
			return out, err
		}
		out.Globals[key] = merged
	}
	for key, raw := range src.Atoms {
		merged, err := mergeValue(raw, dst.Atoms[key])
		if err != nil {
			return out, err
		}
		out.Atoms[key] = merged
	}
	return out, nil
}
func clone(o object) object {
	out := object{}
	for k, v := range o {
		out[k] = v
	}
	return out
}
func checkProjects(dst, src object) error {
	source, ok := decodeObject(src["local-projects"])
	if !ok {
		return ErrUnknownFormat
	}
	target, ok := decodeObject(dst["local-projects"])
	if !ok {
		return ErrUnknownFormat
	}
	for id, raw := range source {
		old, found := target[id]
		if !found {
			continue
		}
		a, aok := decodeObject(raw)
		b, bok := decodeObject(old)
		if !aok || !bok || a == nil || b == nil {
			return ErrUnknownFormat
		}
		if cloudProject(id, b) || !sameJSON(a["rootPaths"], b["rootPaths"]) || !sameJSON(a["id"], b["id"]) {
			return ErrConflict
		}
	}
	return nil
}
func mergeHosts(src, dst json.RawMessage) (json.RawMessage, error) {
	a, aok := decodeObject(src)
	b, bok := decodeObject(dst)
	if !aok || !bok {
		return nil, ErrUnknownFormat
	}
	b = orEmpty(b)
	for host, mapping := range a {
		if err := checkMappings(mapping, b[host]); err != nil {
			return nil, err
		}
		merged, err := mergeMap(mapping, b[host], true)
		if err != nil {
			return nil, err
		}
		b[host] = merged
	}
	return encode(b), nil
}
func mergeMap(src, dst json.RawMessage, conflict bool) (json.RawMessage, error) {
	a, aok := decodeObject(src)
	b, bok := decodeObject(dst)
	if !aok || !bok || a == nil {
		return nil, ErrUnknownFormat
	}
	b = orEmpty(b)
	for k, v := range a {
		if old, exists := b[k]; exists {
			if conflict && !sameJSON(old, v) {
				return nil, ErrConflict
			}
			continue
		}
		b[k] = v
	}
	return encode(b), nil
}
func mergeValue(src, dst json.RawMessage) (json.RawMessage, error) {
	if dst == nil {
		return src, nil
	}
	if a, ok := decodeObject(src); ok && a != nil {
		return mergeMap(src, dst, false)
	}
	var a, b []json.RawMessage
	if json.Unmarshal(src, &a) != nil || a == nil {
		return nil, ErrUnknownFormat
	}
	if json.Unmarshal(dst, &b) != nil || b == nil {
		return nil, ErrUnknownFormat
	}
	for _, v := range a {
		found := false
		for _, old := range b {
			if sameJSON(v, old) {
				found = true
				break
			}
		}
		if !found {
			b = append(b, v)
		}
	}
	return encode(b), nil
}

func checkThreadOwnership(dst, src object) error {
	var projectless []string
	if raw := dst["projectless-thread-ids"]; raw != nil && json.Unmarshal(raw, &projectless) != nil {
		return ErrUnknownFormat
	}
	assignments, ok := decodeObject(src["thread-project-assignments"])
	if !ok {
		return ErrUnknownFormat
	}
	for _, id := range projectless {
		if assignments[id] != nil {
			return ErrConflict
		}
	}
	return nil
}

func checkMappings(src, dst json.RawMessage) error {
	a, aok := decodeObject(src)
	b, bok := decodeObject(dst)
	if !aok || !bok {
		return ErrUnknownFormat
	}
	owners := map[string]string{}
	for id, raw := range b {
		var server string
		if json.Unmarshal(raw, &server) != nil || server == "" {
			return ErrUnknownFormat
		}
		if old, found := owners[server]; found && old != id {
			return ErrConflict
		}
		owners[server] = id
	}
	for id, raw := range a {
		var server string
		if json.Unmarshal(raw, &server) != nil || server == "" {
			return ErrUnknownFormat
		}
		if old, found := owners[server]; found && old != id {
			return ErrConflict
		}
		owners[server] = id
	}
	return nil
}
