package api

import (
	"context"
	"errors"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type billEventDetails struct {
	Amount           *float64
	AmountIsEstimate bool
	PayerID          *uuid.UUID
	PayerName        *string
	PayerInitials    *string
	PayerAvatarURL   *string
}

func (s *server) calendarType(ctx context.Context, tx pgx.Tx, calendarID uuid.UUID) (string, error) {
	var calendarType string
	if err := tx.QueryRow(ctx, `SELECT calendar_type FROM calendars WHERE id=$1`, calendarID).Scan(&calendarType); err != nil {
		return "", err
	}
	return calendarType, nil
}

func normalizeBillAmount(amount *float64) *float64 {
	if amount == nil {
		return nil
	}
	value := math.Round(*amount*100) / 100
	return &value
}

func (s *server) saveBillDetails(ctx context.Context, tx pgx.Tx, eventID, calendarID uuid.UUID, in eventInput) error {
	calendarType, err := s.calendarType(ctx, tx, calendarID)
	if err != nil {
		return err
	}
	if calendarType != "bill_pay" {
		_, err = tx.Exec(ctx, `DELETE FROM bill_event_details WHERE event_id=$1`, eventID)
		return err
	}

	amount := normalizeBillAmount(in.BillAmount)
	if amount == nil && in.BillPayerUserID == nil && !in.BillAmountIsEstimate {
		_, err = tx.Exec(ctx, `DELETE FROM bill_event_details WHERE event_id=$1`, eventID)
		return err
	}

	_, err = tx.Exec(ctx, `INSERT INTO bill_event_details(event_id,amount_due,amount_is_estimate,payer_user_id,updated_at)
		VALUES($1,$2,$3,$4,now())
		ON CONFLICT(event_id) DO UPDATE
		SET amount_due=EXCLUDED.amount_due,
			amount_is_estimate=EXCLUDED.amount_is_estimate,
			payer_user_id=EXCLUDED.payer_user_id,
			updated_at=now()`,
		eventID, amount, in.BillAmountIsEstimate, in.BillPayerUserID)
	return err
}

func (s *server) eventBillDetails(ctx context.Context, eventID uuid.UUID) billEventDetails {
	var out billEventDetails
	err := s.db.QueryRow(ctx, `SELECT
		b.amount_due::double precision,b.amount_is_estimate,b.payer_user_id,
		u.display_name,u.initials,u.avatar_url
		FROM bill_event_details b
		LEFT JOIN users u ON u.id=b.payer_user_id
		WHERE b.event_id=$1`, eventID).Scan(
		&out.Amount, &out.AmountIsEstimate, &out.PayerID,
		&out.PayerName, &out.PayerInitials, &out.PayerAvatarURL,
	)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return billEventDetails{}
	}
	return out
}

func addBillFields(target map[string]any, prefix string, details billEventDetails) {
	target[prefix+"bill_amount"] = details.Amount
	target[prefix+"bill_amount_is_estimate"] = details.AmountIsEstimate
	if details.PayerID == nil {
		target[prefix+"bill_payer"] = nil
		return
	}
	target[prefix+"bill_payer"] = map[string]any{
		"id":           details.PayerID,
		"display_name": details.PayerName,
		"initials":     details.PayerInitials,
		"avatar_url":   details.PayerAvatarURL,
	}
}
