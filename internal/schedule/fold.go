package schedule

import (
	"encoding/json"

	"github.com/GrayCodeAI/graycode-cli/internal/eventlog"
)

// Fold folds a sequence of eventlog events into a map of active ScheduleItems.
func Fold(events []eventlog.Event) map[string]Item {
	items := make(map[string]Item)

	for _, ev := range events {
		switch ev.Type {
		case eventlog.ScheduleCreate:
			var fact eventlog.ScheduleCreateFact
			if err := decodeFact(ev.Data, &fact); err == nil && fact.ID != "" {
				items[fact.ID] = Item{
					ID:        fact.ID,
					Prompt:    fact.Prompt,
					DueAt:     fact.DueAt,
					Interval:  fact.Interval,
					Recurring: fact.Recurring,
					CreatedAt: ev.At,
					UpdatedAt: ev.At,
					Deleted:   false,
				}
			}

		case eventlog.ScheduleUpdate:
			var fact eventlog.ScheduleUpdateFact
			if err := decodeFact(ev.Data, &fact); err == nil && fact.ID != "" {
				if item, exists := items[fact.ID]; exists && !item.Deleted {
					if fact.Prompt != nil {
						item.Prompt = *fact.Prompt
					}
					if fact.DueAt != nil {
						item.DueAt = *fact.DueAt
					}
					item.UpdatedAt = ev.At
					items[fact.ID] = item
				}
			}

		case eventlog.ScheduleDelete:
			var fact eventlog.ScheduleDeleteFact
			if err := decodeFact(ev.Data, &fact); err == nil && fact.ID != "" {
				if item, exists := items[fact.ID]; exists {
					item.Deleted = true
					item.UpdatedAt = ev.At
					items[fact.ID] = item
				}
			}

		case eventlog.ScheduleDue:
			var fact eventlog.ScheduleDueFact
			if err := decodeFact(ev.Data, &fact); err == nil && fact.ID != "" {
				if item, exists := items[fact.ID]; exists && !item.Deleted {
					deliveredAt := fact.DeliveredAt
					if deliveredAt.IsZero() {
						deliveredAt = ev.At
					}
					item.LastDueAt = &deliveredAt
					item.UpdatedAt = ev.At

					if fact.NextDueAt != nil && !fact.NextDueAt.IsZero() {
						item.DueAt = *fact.NextDueAt
					} else if !item.Recurring {
						// One-shot schedule marked completed/deleted
						item.Deleted = true
					}
					items[fact.ID] = item
				}
			}
		}
	}

	// Filter out deleted/completed items
	active := make(map[string]Item)
	for id, item := range items {
		if !item.Deleted {
			active[id] = item
		}
	}
	return active
}

func decodeFact(src any, dst any) error {
	if src == nil {
		return nil
	}
	switch s := src.(type) {
	case eventlog.ScheduleCreateFact:
		if d, ok := dst.(*eventlog.ScheduleCreateFact); ok {
			*d = s
			return nil
		}
	case eventlog.ScheduleUpdateFact:
		if d, ok := dst.(*eventlog.ScheduleUpdateFact); ok {
			*d = s
			return nil
		}
	case eventlog.ScheduleDeleteFact:
		if d, ok := dst.(*eventlog.ScheduleDeleteFact); ok {
			*d = s
			return nil
		}
	case eventlog.ScheduleDueFact:
		if d, ok := dst.(*eventlog.ScheduleDueFact); ok {
			*d = s
			return nil
		}
	}

	// Fallback to JSON conversion for map[string]any or wire payloads
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
