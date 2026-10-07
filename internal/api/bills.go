package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
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

type billPaymentEntry struct {
	ID               uuid.UUID
	AmountPaid       float64
	PaidOn           string
	ClearedOn        *string
	SettlesBill      bool
	RecordedAt       time.Time
	PaidByID         *uuid.UUID
	PaidByName       *string
	PaidByInitials   *string
	PaidByAvatarURL  *string
}

type billPaymentDetails struct {
	Status               string
	NoBalance            bool
	NoBalanceMarkedAt    *time.Time
	AmountPaid           *float64
	PaidAt               *time.Time
	PaidOn               *string
	ClearedOn            *string
	PaidByID             *uuid.UUID
	PaidByName           *string
	PaidByInitials       *string
	PaidByAvatarURL      *string
	AllocatedAmount      *float64
	AllocatedOn          *string
	AllocatedAt          *time.Time
	AllocatedByID        *uuid.UUID
	AllocatedByName      *string
	AllocatedByInitials  *string
	AllocatedByAvatarURL *string
	Payments             []billPaymentEntry
}

type billOccurrenceTarget struct {
	EventID          uuid.UUID
	EffectiveEventID uuid.UUID
	CalendarID       uuid.UUID
	Title            string
	CalendarType     string
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
		if _, err = tx.Exec(ctx, `DELETE FROM bill_event_details WHERE event_id=$1`, eventID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM bill_payments WHERE event_id=$1`, eventID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM bill_allocations WHERE event_id=$1`, eventID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM bill_no_balance_occurrences WHERE event_id=$1`, eventID)
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

func billPaymentPerson(id *uuid.UUID, name, initials, avatar *string) any {
	if id == nil {
		return nil
	}
	return map[string]any{
		"id":           id,
		"display_name": name,
		"initials":     initials,
		"avatar_url":   avatar,
	}
}

func billPaymentEntryJSON(entry billPaymentEntry) map[string]any {
	return map[string]any{
		"id":           entry.ID,
		"amount_paid":  entry.AmountPaid,
		"paid_on":      entry.PaidOn,
		"cleared_on":   entry.ClearedOn,
		"settles_bill": entry.SettlesBill,
		"recorded_at":  entry.RecordedAt,
		"paid_by":      billPaymentPerson(entry.PaidByID, entry.PaidByName, entry.PaidByInitials, entry.PaidByAvatarURL),
	}
}

func (s *server) eventBillPayment(ctx context.Context, eventID uuid.UUID, occurrenceStart time.Time) billPaymentDetails {
	out := billPaymentDetails{Status: "due", Payments: []billPaymentEntry{}}

	var allocatedAmount float64
	var allocatedOn string
	var allocatedAt time.Time
	err := s.db.QueryRow(ctx, `SELECT
		a.amount_allocated::double precision,to_char(a.allocated_on,'YYYY-MM-DD'),a.allocated_at,
		a.allocated_by_user_id,u.display_name,u.initials,u.avatar_url
		FROM bill_allocations a
		LEFT JOIN users u ON u.id=a.allocated_by_user_id
		WHERE a.event_id=$1 AND a.occurrence_start=$2`, eventID, occurrenceStart).Scan(
		&allocatedAmount, &allocatedOn, &allocatedAt,
		&out.AllocatedByID, &out.AllocatedByName, &out.AllocatedByInitials, &out.AllocatedByAvatarURL,
	)
	if err == nil {
		out.AllocatedAmount = &allocatedAmount
		out.AllocatedOn = &allocatedOn
		out.AllocatedAt = &allocatedAt
		out.Status = "allocated"
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out
	}

	var markedAt time.Time
	err = s.db.QueryRow(ctx, `SELECT marked_at FROM bill_no_balance_occurrences
		WHERE event_id=$1 AND occurrence_start=$2`, eventID, occurrenceStart).Scan(&markedAt)
	if err == nil {
		out.NoBalance = true
		out.NoBalanceMarkedAt = &markedAt
		out.Status = "no_balance"
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out
	}

	rows, err := s.db.Query(ctx, `SELECT
		p.id,p.amount_paid::double precision,to_char(p.paid_on,'YYYY-MM-DD'),
		CASE WHEN p.cleared_on IS NULL THEN NULL ELSE to_char(p.cleared_on,'YYYY-MM-DD') END,
		p.settles_bill,p.paid_at,p.paid_by_user_id,u.display_name,u.initials,u.avatar_url
		FROM bill_payments p
		LEFT JOIN users u ON u.id=p.paid_by_user_id
		WHERE p.event_id=$1 AND p.occurrence_start=$2
		ORDER BY p.paid_on,p.paid_at,p.id`, eventID, occurrenceStart)
	if err != nil {
		return out
	}
	defer rows.Close()

	total := 0.0
	var final *billPaymentEntry
	var last *billPaymentEntry
	allCleared := true
	for rows.Next() {
		var entry billPaymentEntry
		if rows.Scan(
			&entry.ID, &entry.AmountPaid, &entry.PaidOn, &entry.ClearedOn,
			&entry.SettlesBill, &entry.RecordedAt, &entry.PaidByID,
			&entry.PaidByName, &entry.PaidByInitials, &entry.PaidByAvatarURL,
		) != nil {
			continue
		}
		total += entry.AmountPaid
		out.Payments = append(out.Payments, entry)
		if entry.ClearedOn == nil {
			allCleared = false
		}
		copy := entry
		last = &copy
		if entry.SettlesBill {
			final = &copy
		}
	}
	if len(out.Payments) == 0 {
		if out.NoBalance {
			out.Status = "no_balance"
		}
		return out
	}

	out.AmountPaid = &total
	if !out.NoBalance {
		out.Status = "partial"
	}
	representative := last
	if final != nil {
		representative = final
		if !out.NoBalance {
			out.Status = "paid"
			if allCleared {
				out.Status = "cleared"
			}
		}
	}
	if representative != nil {
		out.PaidAt = &representative.RecordedAt
		out.PaidOn = &representative.PaidOn
		out.ClearedOn = representative.ClearedOn
		out.PaidByID = representative.PaidByID
		out.PaidByName = representative.PaidByName
		out.PaidByInitials = representative.PaidByInitials
		out.PaidByAvatarURL = representative.PaidByAvatarURL
	}
	return out
}

func addBillPaymentFields(target map[string]any, details billPaymentDetails) {
	target["bill_payment_status"] = details.Status
	target["bill_paid"] = details.Status == "paid" || details.Status == "cleared"
	target["bill_no_balance"] = details.NoBalance
	target["bill_amount_paid"] = details.AmountPaid
	target["bill_amount_allocated"] = details.AllocatedAmount
	target["bill_allocated_on"] = details.AllocatedOn
	target["bill_allocated_at"] = details.AllocatedAt
	target["bill_allocated_by"] = billPaymentPerson(
		details.AllocatedByID, details.AllocatedByName, details.AllocatedByInitials, details.AllocatedByAvatarURL,
	)
	target["bill_paid_at"] = details.PaidAt
	target["bill_paid_on"] = details.PaidOn
	target["bill_cleared_on"] = details.ClearedOn
	target["bill_no_balance_marked_at"] = details.NoBalanceMarkedAt
	target["bill_payment_count"] = len(details.Payments)

	payments := make([]map[string]any, 0, len(details.Payments))
	for _, entry := range details.Payments {
		payments = append(payments, billPaymentEntryJSON(entry))
	}
	target["bill_payments"] = payments
	target["bill_paid_by"] = billPaymentPerson(
		details.PaidByID, details.PaidByName, details.PaidByInitials, details.PaidByAvatarURL,
	)
}

func (s *server) billOccurrenceTarget(r *http.Request, eventID uuid.UUID, occurrenceStart time.Time) (billOccurrenceTarget, error) {
	out := billOccurrenceTarget{EventID: eventID, EffectiveEventID: eventID}
	var seriesStart time.Time
	var recurring bool
	if err := s.db.QueryRow(r.Context(), `SELECT e.calendar_id,e.title,c.calendar_type,e.starts_at,
		EXISTS(SELECT 1 FROM event_recurrence er WHERE er.event_id=e.id)
		FROM events e JOIN calendars c ON c.id=e.calendar_id
		WHERE e.id=$1`, eventID).Scan(
		&out.CalendarID, &out.Title, &out.CalendarType, &seriesStart, &recurring,
	); err != nil {
		return out, errors.New("Bill not found")
	}

	if recurring {
		_, exists, err := s.validSeriesOccurrence(r.Context(), eventID, occurrenceStart)
		if err != nil {
			return out, errors.New("Could not validate this bill occurrence")
		}
		if !exists {
			return out, errors.New("That bill occurrence is not part of this recurring bill")
		}
	} else if !seriesStart.Equal(occurrenceStart) {
		return out, errors.New("That bill occurrence does not match this bill")
	}

	var replacementID uuid.UUID
	err := s.db.QueryRow(r.Context(), `SELECT replacement_event_id
		FROM event_occurrence_exceptions
		WHERE event_id=$1 AND original_start=$2 AND replacement_event_id IS NOT NULL`,
		eventID, occurrenceStart).Scan(&replacementID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, errors.New("Could not check this bill occurrence")
	}
	if replacementID != uuid.Nil {
		out.EffectiveEventID = replacementID
		if err := s.db.QueryRow(r.Context(), `SELECT e.calendar_id,e.title,c.calendar_type
			FROM events e JOIN calendars c ON c.id=e.calendar_id
			WHERE e.id=$1`, replacementID).Scan(&out.CalendarID, &out.Title, &out.CalendarType); err != nil {
			return out, errors.New("Could not load this bill occurrence")
		}
	}
	if out.CalendarType != "bill_pay" {
		return out, errors.New("This event is not on a Bill Pay calendar")
	}
	if !s.canEditCalendar(r, out.CalendarID) {
		return out, errors.New("You cannot update this bill")
	}
	return out, nil
}

func parseBillDate(value string, required bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return nil, errors.New("Choose the payment date")
		}
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, errors.New("Check the payment date")
	}
	return &t, nil
}

func validateBillPaymentInput(amount float64, paidOn string, clearedOn string) (*time.Time, *time.Time, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 || amount > 9999999999.99 {
		return nil, nil, errors.New("Enter the amount paid")
	}
	paid, err := parseBillDate(paidOn, true)
	if err != nil {
		return nil, nil, err
	}
	cleared, err := parseBillDate(clearedOn, false)
	if err != nil {
		return nil, nil, err
	}
	if cleared != nil && cleared.Before(*paid) {
		return nil, nil, errors.New("Cleared date cannot be before the payment date")
	}
	return paid, cleared, nil
}

func (s *server) activeBillPayer(ctx context.Context, requested *uuid.UUID, fallback uuid.UUID) (uuid.UUID, error) {
	paidBy := fallback
	if requested != nil && *requested != uuid.Nil {
		paidBy = *requested
	}
	var active bool
	if err := s.db.QueryRow(ctx, `SELECT active FROM users WHERE id=$1`, paidBy).Scan(&active); err != nil || !active {
		return uuid.Nil, errors.New("Choose an active person who made this payment")
	}
	return paidBy, nil
}

func (s *server) setBillAllocation(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid bill")
		return
	}
	var in struct {
		OccurrenceStart time.Time `json:"occurrence_start"`
		Allocated       bool      `json:"allocated"`
		AmountAllocated float64   `json:"amount_allocated"`
		AllocatedOn     string    `json:"allocated_on"`
	}
	if decode(r, &in) != nil || in.OccurrenceStart.IsZero() {
		writeError(w, 400, "Check the allocation details")
		return
	}
	target, err := s.billOccurrenceTarget(r, eventID, in.OccurrenceStart)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	if !in.Allocated {
		if _, err := s.db.Exec(r.Context(), `DELETE FROM bill_allocations
			WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
			writeError(w, 500, "Could not clear allocation")
			return
		}
		s.audit(r, "bill_allocation_clear", "event", &eventID, "Cleared bill allocation "+cleanText(target.Title, 200), map[string]any{
			"occurrence_start": in.OccurrenceStart,
		})
		out := map[string]any{}
		addBillPaymentFields(out, s.eventBillPayment(r.Context(), eventID, in.OccurrenceStart))
		writeJSON(w, 200, out)
		return
	}

	if math.IsNaN(in.AmountAllocated) || math.IsInf(in.AmountAllocated, 0) || in.AmountAllocated <= 0 || in.AmountAllocated > 9999999999.99 {
		writeError(w, 400, "Enter the amount allocated")
		return
	}
	allocatedOn, err := parseBillDate(in.AllocatedOn, true)
	if err != nil {
		writeError(w, 400, "Choose the allocation date")
		return
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO bill_allocations(
			event_id,occurrence_start,amount_allocated,allocated_on,allocated_by_user_id,allocated_at,updated_at
		) VALUES($1,$2,$3,$4,$5,now(),now())
		ON CONFLICT(event_id,occurrence_start) DO UPDATE SET
			amount_allocated=EXCLUDED.amount_allocated,
			allocated_on=EXCLUDED.allocated_on,
			allocated_by_user_id=EXCLUDED.allocated_by_user_id,
			updated_at=now()`,
		eventID, in.OccurrenceStart, normalizeBillAmount(&in.AmountAllocated), allocatedOn, currentActor(r).ID); err != nil {
		writeError(w, 500, "Could not save allocation")
		return
	}
	s.audit(r, "bill_allocation", "event", &eventID, "Allocated bill funds "+cleanText(target.Title, 200), map[string]any{
		"occurrence_start": in.OccurrenceStart, "amount_allocated": in.AmountAllocated, "allocated_on": in.AllocatedOn,
	})
	out := map[string]any{}
	addBillPaymentFields(out, s.eventBillPayment(r.Context(), eventID, in.OccurrenceStart))
	writeJSON(w, 200, out)
}

func (s *server) addBillPayment(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid bill")
		return
	}
	var in struct {
		OccurrenceStart time.Time  `json:"occurrence_start"`
		PaidByUserID    *uuid.UUID `json:"paid_by_user_id,omitempty"`
		AmountPaid      float64    `json:"amount_paid"`
		PaidOn          string     `json:"paid_on"`
		ClearedOn       string     `json:"cleared_on,omitempty"`
		SettlesBill     bool       `json:"settles_bill"`
	}
	if decode(r, &in) != nil || in.OccurrenceStart.IsZero() {
		writeError(w, 400, "Check the payment details")
		return
	}
	paidOn, clearedOn, err := validateBillPaymentInput(in.AmountPaid, in.PaidOn, in.ClearedOn)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	target, err := s.billOccurrenceTarget(r, eventID, in.OccurrenceStart)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	paidBy, err := s.activeBillPayer(r.Context(), in.PaidByUserID, currentActor(r).ID)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not save payment")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `DELETE FROM bill_no_balance_occurrences
		WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
		writeError(w, 500, "Could not reopen this bill")
		return
	}
	var paymentID uuid.UUID
	if err = tx.QueryRow(r.Context(), `INSERT INTO bill_payments(
			event_id,occurrence_start,paid_by_user_id,amount_paid,paid_on,cleared_on,settles_bill,paid_at,updated_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,now(),now()) RETURNING id`,
		eventID, in.OccurrenceStart, paidBy, normalizeBillAmount(&in.AmountPaid), paidOn, clearedOn, in.SettlesBill).Scan(&paymentID); err != nil {
		writeError(w, 500, "Could not save payment")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not save payment")
		return
	}
	s.audit(r, "bill_payment_add", "event", &eventID, "Recorded bill payment "+cleanText(target.Title, 200), map[string]any{
		"occurrence_start": in.OccurrenceStart, "payment_id": paymentID, "paid_by_user_id": paidBy,
		"amount_paid": in.AmountPaid, "paid_on": in.PaidOn, "cleared_on": in.ClearedOn, "settles_bill": in.SettlesBill,
	})
	out := map[string]any{}
	addBillPaymentFields(out, s.eventBillPayment(r.Context(), eventID, in.OccurrenceStart))
	writeJSON(w, 201, out)
}

func (s *server) updateBillPayment(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid bill")
		return
	}
	paymentID, err := uuid.Parse(r.PathValue("payment_id"))
	if err != nil {
		writeError(w, 400, "Invalid payment")
		return
	}
	var in struct {
		OccurrenceStart time.Time  `json:"occurrence_start"`
		PaidByUserID    *uuid.UUID `json:"paid_by_user_id,omitempty"`
		AmountPaid      float64    `json:"amount_paid"`
		PaidOn          string     `json:"paid_on"`
		ClearedOn       string     `json:"cleared_on,omitempty"`
		SettlesBill     bool       `json:"settles_bill"`
	}
	if decode(r, &in) != nil || in.OccurrenceStart.IsZero() {
		writeError(w, 400, "Check the payment details")
		return
	}
	paidOn, clearedOn, err := validateBillPaymentInput(in.AmountPaid, in.PaidOn, in.ClearedOn)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	target, err := s.billOccurrenceTarget(r, eventID, in.OccurrenceStart)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	paidBy, err := s.activeBillPayer(r.Context(), in.PaidByUserID, currentActor(r).ID)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	tag, err := s.db.Exec(r.Context(), `UPDATE bill_payments SET
		paid_by_user_id=$4,amount_paid=$5,paid_on=$6,cleared_on=$7,settles_bill=$8,updated_at=now()
		WHERE id=$1 AND event_id=$2 AND occurrence_start=$3`,
		paymentID, eventID, in.OccurrenceStart, paidBy, normalizeBillAmount(&in.AmountPaid), paidOn, clearedOn, in.SettlesBill)
	if err != nil {
		writeError(w, 500, "Could not update payment")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Payment not found")
		return
	}
	s.audit(r, "bill_payment_update", "event", &eventID, "Updated bill payment "+cleanText(target.Title, 200), map[string]any{
		"occurrence_start": in.OccurrenceStart, "payment_id": paymentID, "paid_by_user_id": paidBy,
		"amount_paid": in.AmountPaid, "paid_on": in.PaidOn, "cleared_on": in.ClearedOn, "settles_bill": in.SettlesBill,
	})
	out := map[string]any{}
	addBillPaymentFields(out, s.eventBillPayment(r.Context(), eventID, in.OccurrenceStart))
	writeJSON(w, 200, out)
}

func (s *server) deleteBillPayment(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid bill")
		return
	}
	paymentID, err := uuid.Parse(r.PathValue("payment_id"))
	if err != nil {
		writeError(w, 400, "Invalid payment")
		return
	}
	var in struct {
		OccurrenceStart time.Time `json:"occurrence_start"`
	}
	if decode(r, &in) != nil || in.OccurrenceStart.IsZero() {
		writeError(w, 400, "Choose the bill occurrence")
		return
	}
	target, err := s.billOccurrenceTarget(r, eventID, in.OccurrenceStart)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	tag, err := s.db.Exec(r.Context(), `DELETE FROM bill_payments
		WHERE id=$1 AND event_id=$2 AND occurrence_start=$3`, paymentID, eventID, in.OccurrenceStart)
	if err != nil {
		writeError(w, 500, "Could not delete payment")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "Payment not found")
		return
	}
	s.audit(r, "bill_payment_delete", "event", &eventID, "Deleted bill payment "+cleanText(target.Title, 200), map[string]any{
		"occurrence_start": in.OccurrenceStart, "payment_id": paymentID,
	})
	out := map[string]any{}
	addBillPaymentFields(out, s.eventBillPayment(r.Context(), eventID, in.OccurrenceStart))
	writeJSON(w, 200, out)
}

func (s *server) setBillNoBalance(w http.ResponseWriter, r *http.Request) {
	eventID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "Invalid bill")
		return
	}
	var in struct {
		OccurrenceStart time.Time `json:"occurrence_start"`
		NoBalance       bool      `json:"no_balance"`
	}
	if decode(r, &in) != nil || in.OccurrenceStart.IsZero() {
		writeError(w, 400, "Choose the bill occurrence")
		return
	}
	target, err := s.billOccurrenceTarget(r, eventID, in.OccurrenceStart)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not update bill status")
		return
	}
	defer tx.Rollback(r.Context())
	if in.NoBalance {
		if _, err = tx.Exec(r.Context(), `DELETE FROM bill_payments
			WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
			writeError(w, 500, "Could not clear existing payments")
			return
		}
		if _, err = tx.Exec(r.Context(), `DELETE FROM bill_allocations
			WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
			writeError(w, 500, "Could not clear existing allocation")
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO bill_no_balance_occurrences(
			event_id,occurrence_start,marked_by_user_id,marked_at
		) VALUES($1,$2,$3,now())
		ON CONFLICT(event_id,occurrence_start) DO UPDATE
		SET marked_by_user_id=EXCLUDED.marked_by_user_id,marked_at=now()`,
			eventID, in.OccurrenceStart, currentActor(r).ID); err != nil {
			writeError(w, 500, "Could not mark no balance")
			return
		}
	} else {
		if _, err = tx.Exec(r.Context(), `DELETE FROM bill_no_balance_occurrences
			WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
			writeError(w, 500, "Could not reopen this bill")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not update bill status")
		return
	}
	action := "bill_reopened"
	message := "Reopened bill " + cleanText(target.Title, 200)
	if in.NoBalance {
		action = "bill_no_balance"
		message = "Marked no balance " + cleanText(target.Title, 200)
	}
	s.audit(r, action, "event", &eventID, message, map[string]any{"occurrence_start": in.OccurrenceStart})
	out := map[string]any{}
	addBillPaymentFields(out, s.eventBillPayment(r.Context(), eventID, in.OccurrenceStart))
	writeJSON(w, 200, out)
}

// Legacy Alpha10 endpoint. It remains available for older clients while the
// web and mobile clients use the multi-payment endpoints above.
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
		writeError(w, 400, "Check the payment details")
		return
	}
	target, err := s.billOccurrenceTarget(r, eventID, in.OccurrenceStart)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}

	if !in.Paid {
		tx, err := s.db.Begin(r.Context())
		if err != nil {
			writeError(w, 500, "Could not mark this bill unpaid")
			return
		}
		defer tx.Rollback(r.Context())
		if _, err = tx.Exec(r.Context(), `DELETE FROM bill_payments WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
			writeError(w, 500, "Could not mark this bill unpaid")
			return
		}
		if _, err = tx.Exec(r.Context(), `DELETE FROM bill_no_balance_occurrences WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
			writeError(w, 500, "Could not mark this bill unpaid")
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			writeError(w, 500, "Could not mark this bill unpaid")
			return
		}
		s.audit(r, "bill_unpaid", "event", &eventID, "Marked bill unpaid "+cleanText(target.Title, 200), map[string]any{"occurrence_start": in.OccurrenceStart})
		writeJSON(w, 200, map[string]any{"bill_payment_status": "due", "bill_paid": false})
		return
	}

	paidBy, err := s.activeBillPayer(r.Context(), in.PaidByUserID, currentActor(r).ID)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	amount := normalizeBillAmount(in.AmountPaid)
	if amount == nil {
		var due *float64
		err = s.db.QueryRow(r.Context(), `SELECT amount_due::double precision FROM bill_event_details WHERE event_id=$1`,
			target.EffectiveEventID).Scan(&due)
		if errors.Is(err, pgx.ErrNoRows) && target.EffectiveEventID != eventID {
			err = s.db.QueryRow(r.Context(), `SELECT amount_due::double precision FROM bill_event_details WHERE event_id=$1`, eventID).Scan(&due)
		}
		if err == nil {
			amount = normalizeBillAmount(due)
		}
	}
	if amount == nil || *amount <= 0 {
		writeError(w, 400, "Enter the amount paid")
		return
	}

	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "Could not mark this bill paid")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `DELETE FROM bill_payments WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
		writeError(w, 500, "Could not update payment")
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM bill_no_balance_occurrences WHERE event_id=$1 AND occurrence_start=$2`, eventID, in.OccurrenceStart); err != nil {
		writeError(w, 500, "Could not update payment")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO bill_payments(
		event_id,occurrence_start,paid_by_user_id,amount_paid,paid_on,settles_bill,paid_at,updated_at
	) VALUES($1,$2,$3,$4,CURRENT_DATE,true,now(),now())`, eventID, in.OccurrenceStart, paidBy, amount); err != nil {
		writeError(w, 500, "Could not mark this bill paid")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "Could not mark this bill paid")
		return
	}
	out := map[string]any{}
	addBillPaymentFields(out, s.eventBillPayment(r.Context(), eventID, in.OccurrenceStart))
	writeJSON(w, 200, out)
}
