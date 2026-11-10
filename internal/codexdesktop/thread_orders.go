package codexdesktop

import "encoding/json"

// Sidebar orders are objects with threadIds and an optional created_at or
// updated_at sortKey; absence of sortKey is meaningful and stays absent.
type threadOrder struct {
	ThreadIDs []string `json:"threadIds"`
	SortKey   *string  `json:"sortKey,omitempty"`
}

func parseThreadOrder(raw json.RawMessage) (threadOrder, error) {
	var out threadOrder
	fields, ok := decodeObject(raw)
	if ok && fields != nil {
		for key := range fields {
			if key != "threadIds" && key != "sortKey" {
				return out, ErrUnknownFormat
			}
		}
		if json.Unmarshal(fields["threadIds"], &out.ThreadIDs) != nil || out.ThreadIDs == nil {
			return out, ErrUnknownFormat
		}
		if sort, found := fields["sortKey"]; found {
			var key string
			if json.Unmarshal(sort, &key) != nil || (key != "created_at" && key != "updated_at") {
				return out, ErrUnknownFormat
			}
			out.SortKey = &key
		}
		return out, nil
	}
	// Earlier releases could write a bare array; normalize that legacy output
	// into an object without inventing a sorting preference.
	if json.Unmarshal(raw, &out.ThreadIDs) != nil || out.ThreadIDs == nil {
		return out, ErrUnknownFormat
	}
	return out, nil
}
func (v *selection) selectOrders(p partition) error {
	orders, ok := decodeObject(p.Globals["sidebar-project-thread-orders"])
	if !ok {
		return ErrUnknownFormat
	}
	selected := object{}
	for project, raw := range orders {
		if !v.projects[project] {
			continue
		}
		order, err := parseThreadOrder(raw)
		if err != nil {
			return err
		}
		filtered := []string{}
		for _, id := range order.ThreadIDs {
			if v.threads[id] && v.threadProjects[id] == project {
				filtered = append(filtered, id)
			}
		}
		order.ThreadIDs = filtered
		selected[project] = encode(order)
	}
	if len(selected) > 0 {
		v.part.Globals["sidebar-project-thread-orders"] = encode(selected)
	}
	return nil
}
func mergeOrders(src, dst json.RawMessage) (json.RawMessage, error) {
	source, sok := decodeObject(src)
	target, tok := decodeObject(dst)
	if !sok || !tok {
		return nil, ErrUnknownFormat
	}
	target = orEmpty(target)
	for project, raw := range source {
		order, err := parseThreadOrder(raw)
		if err != nil {
			return nil, err
		}
		if previous, found := target[project]; found {
			old, err := parseThreadOrder(previous)
			if err != nil {
				return nil, err
			}
			old.ThreadIDs = appendUniqueIDs(old.ThreadIDs, order.ThreadIDs)
			order = old
		}
		target[project] = encode(order)
	}
	return encode(target), nil
}
func appendUniqueIDs(target, source []string) []string {
	out := append([]string{}, target...)
	seen := map[string]bool{}
	for _, id := range target {
		seen[id] = true
	}
	for _, id := range source {
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out
}
