package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

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

type billPaymentDetails struct {
	AmountPaid      *float64
	PaidAt          *time.Time
	PaidByID        *uuid.UUID
	PaidByName      *string
	PaidByInitials  *string
	PaidByAvatarURL *string
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


func (s *server) eventBillPayment(ctx context.Context, eventID uuid.UUID, occurrenceStart time.Time) billPaymentDetails {
	var out billPaymentDetails
	err := s.db.QueryRow(ctx, `SELECT
		p.amount_paid::double precision,p.paid_at,p.paid_by_user_id,
		u.display_name,u.initials,u.avatar_url
		FROM bill_payments p
		LEFT JOIN users u ON u.id=p.paid_by_user_id
		WHERE p.event_id=$1 AND p.occurrence_start=$2`, eventID, occurrenceStart).Scan(
		&out.AmountPaid, &out.PaidAt, &out.PaidByID,
		&out.PaidByName, &out.PaidByInitials, &out.PaidByAvatarURL,
	)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return billPaymentDetails{}
	}
	return out
}

func addBillPaymentFields(target map[string]any, details billPaymentDetails) {
	target["bill_paid"] = details.PaidAt != nil
	target["bill_amount_paid"] = details.AmountPaid
	target["bill_paid_at"] = details.PaidAt
	if details.PaidByID == nil {
		target["bill_paid_by"] = nil
		return
	}
	target["bill_paid_by"] = map[string]any{
		"id":           details.PaidByID,
		"display_name": details.PaidByName,
		"initials":     details.PaidByInitials,
		"avatar_url":   details.PaidByAvatarURL,
	}
}

func (s *server) setBillPayment(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid bill")
		return
	}
	var in struct {
		OccurrenceStart time.Time  `json:"occurrence_start"`
		Paid            bool       `json:"paid"`
		PaidByUserID    *uuid.UUID `json:"paid_by_user_id,omitempty"`
		AmountPaid      *float64   `json:"amount_paid,omitempty"`
	}
	if decode(r, &in) != nil || in.OccurrenceStart.IsZero() {
		writeError(w, 400, "Choose the bill occurrence")
		return
	}
	if in.AmountPaid != nil {
		if math.IsNaN(*in.AmountPaid) || math.IsInf(*in.AmountPaid, 0) || *in.AmountPaid < 0 || *in.AmountPaid > 9999999999.99 {
			writeError(w, 400, "Check the amount paid")
			return
		}
	}

	var effectiveEventID, calendarID uuid.UUID
	var title, calendarType string
	if err = s.db.QueryRow(r.Context(), `SELECT e.id,e.calendar_id,e.title,c.calendar_type
		FROM events e JOIN calendars c ON c.id=e.calendar_id
		WHERE e.id=$1`, eventID).Scan(&effectiveEventID, &calendarID, &title, &calendarType); err != nil {
		writeError(w, 404, "Bill not found")
		return
	}

	var replacementID uuid.UUID
	if err = s.db.QueryRow(r.Context(), `SELECT replacement_event_id
		FROM event_occurrence_exceptions
		WHERE event_id=$1 AND original_start=$2 AND replacement_event_id IS NOT NULL`,
		eventID, in.OccurrenceStart).Scan(&replacementID); err != nil && err != pgx.ErrNoRows {
		writeError(w, 500, "Could not check this bill occurrence")
		return
	}
	if replacementID != uuid.Nil {
		effectiveEventID = replacementID
		if err = s.db.QueryRow(r.Context(), `SELECT e.calendar_id,e.title,c.calendar_type
			FROM events e JOIN calendars c ON c.id=e.calendar_id
			WHERE e.id=$1`, replacementID).Scan(&calendarID, &title, &calendarType); err != nil {
			writeError(w, 500, "Could not load this bill occurrence")
			return
		}
	}
	if calendarType != "bill_pay" {
		writeError(w, 400, "This event is not on a Bill Pay calendar")
		return
	}
	if !s.canEditCalendar(r, calendarID) {
		writeError(w, 403, "You cannot update this bill")
		return
	}

	if !in.Paid {
		if _, err = s.db.Exec(r.Context(), `DELETE FROM bill_payments WHERE event_id=$1 AND occurrence_start=$2`,
			eventID, in.OccurrenceStart); err != nil {
			writeError(w, 500, "Could not mark this bill unpaid")
			return
		}
		s.audit(r, "bill_unpaid", "event", &eventID, "Marked bill unpaid "+cleanText(title, 200), map[string]any{
			"occurrence_start": in.OccurrenceStart,
		})
		writeJSON(w, 200, map[string]any{"paid": false})
		return
	}

	paidBy := currentActor(r).ID
	if in.PaidByUserID != nil && *in.PaidByUserID != uuid.Nil {
		paidBy = *in.PaidByUserID
	}
	var active bool
	if err = s.db.QueryRow(r.Context(), `SELECT active FROM users WHERE id=$1`, paidBy).Scan(&active); err != nil || !active {
		writeError(w, 400, "Choose an active person who paid this bill")
		return
	}

	amount := normalizeBillAmount(in.AmountPaid)
	if amount == nil {
		var due *float64
		err = s.db.QueryRow(r.Context(), `SELECT amount_due::double precision FROM bill_event_details WHERE event_id=$1`,
			effectiveEventID).Scan(&due)
		if err == pgx.ErrNoRows && effectiveEventID != eventID {
			err = s.db.QueryRow(r.Context(), `SELECT amount_due::double precision FROM bill_event_details WHERE event_id=$1`,
				eventID).Scan(&due)
		}
		if err == nil {
			amount = normalizeBillAmount(due)
		}
	}

	_, err = s.db.Exec(r.Context(), `INSERT INTO bill_payments(
			event_id,occurrence_start,paid_by_user_id,amount_paid,paid_at,updated_at
		) VALUES($1,$2,$3,$4,now(),now())
		ON CONFLICT(event_id,occurrence_start) DO UPDATE
		SET paid_by_user_id=EXCLUDED.paid_by_user_id,
			amount_paid=EXCLUDED.amount_paid,
			updated_at=now()`,
		eventID, in.OccurrenceStart, paidBy, amount)
	if err != nil {
		writeError(w, 500, "Could not mark this bill paid")
		return
	}

	s.audit(r, "bill_paid", "event", &eventID, "Marked bill paid "+cleanText(title, 200), map[string]any{
		"occurrence_start": in.OccurrenceStart, "paid_by_user_id": paidBy, "amount_paid": amount,
	})
	details := s.eventBillPayment(r.Context(), eventID, in.OccurrenceStart)
	out := map[string]any{}
	addBillPaymentFields(out, details)
	writeJSON(w, 200, out)
}
