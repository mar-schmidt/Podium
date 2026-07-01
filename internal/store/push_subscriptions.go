package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// UpsertPushSubscription registers (or refreshes) a push destination keyed by
// its endpoint. Re-subscribing the same client updates the stored payload
// rather than creating a duplicate row.
func (s *Store) UpsertPushSubscription(ctx context.Context, sub PushSubscription) (PushSubscription, error) {
	if sub.ID == "" {
		sub.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO push_subscriptions
		(id, kind, endpoint, payload)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(endpoint) DO UPDATE SET kind = excluded.kind, payload = excluded.payload`,
		sub.ID, sub.Kind, sub.Endpoint, sub.Payload,
	)
	if err != nil {
		return PushSubscription{}, fmt.Errorf("upsert push subscription %q: %w", sub.Endpoint, err)
	}
	return s.getPushSubscriptionByEndpoint(ctx, sub.Endpoint)
}

// ListPushSubscriptions returns all subscriptions for a delivery kind (e.g.
// "webpush"). An empty kind returns every subscription.
func (s *Store) ListPushSubscriptions(ctx context.Context, kind string) ([]PushSubscription, error) {
	query := pushSubscriptionSelect
	var args []any
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY created_at ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions (%q): %w", kind, err)
	}
	defer rows.Close()

	var subs []PushSubscription
	for rows.Next() {
		sub, err := scanPushSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// DeletePushSubscriptionByEndpoint removes a subscription. It is idempotent:
// deleting an already-absent endpoint is not an error, which suits pruning dead
// subscriptions reported by the push service.
func (s *Store) DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint); err != nil {
		return fmt.Errorf("delete push subscription %q: %w", endpoint, err)
	}
	return nil
}

func (s *Store) getPushSubscriptionByEndpoint(ctx context.Context, endpoint string) (PushSubscription, error) {
	row := s.db.QueryRowContext(ctx, pushSubscriptionSelect+` WHERE endpoint = ?`, endpoint)
	sub, err := scanPushSubscription(row)
	if err != nil {
		return PushSubscription{}, fmt.Errorf("get push subscription %q: %w", endpoint, err)
	}
	return sub, nil
}

const pushSubscriptionSelect = `SELECT id, kind, endpoint, payload, created_at FROM push_subscriptions`

func scanPushSubscription(row scanner) (PushSubscription, error) {
	var sub PushSubscription
	if err := row.Scan(&sub.ID, &sub.Kind, &sub.Endpoint, &sub.Payload, &sub.CreatedAt); err != nil {
		return PushSubscription{}, err
	}
	return sub, nil
}
