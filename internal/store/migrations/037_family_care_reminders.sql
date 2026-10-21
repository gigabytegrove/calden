-- 037: recurring school, home maintenance and pet-care reminders.
-- Reuses notifications, existing inbox and push outbox. No second delivery service.
ALTER TABLE notifications
 DROP CONSTRAINT IF EXISTS notifications_kind_check;
ALTER TABLE notifications
 ADD CONSTRAINT notifications_kind_check
 CHECK (kind IN ('event_assigned','event_family','bill_review','care_due'));
ALTER TABLE notifications
 ADD COLUMN IF NOT EXISTS care_item_id UUID REFERENCES family_care_items(id) ON DELETE CASCADE;
CREATE UNIQUE INDEX IF NOT EXISTS notification_care_occurrence_once
 ON notifications(user_id,care_item_id,kind,occurrence_start)
 WHERE kind='care_due' AND care_item_id IS NOT NULL AND occurrence_start IS NOT NULL;

-- Expand previously persisted preferences without altering a person's opt-outs
-- of event or bill notifications. Existing opt-ins become opt-ins for care due
-- only if the person already enabled all default notification kinds.
ALTER TABLE member_notification_preferences
 DROP CONSTRAINT IF EXISTS member_notification_preferences_enabled_kinds_check;
ALTER TABLE member_notification_preferences
 ADD CONSTRAINT member_notification_preferences_enabled_kinds_check
 CHECK(enabled_kinds <@ ARRAY['event_assigned','event_family','bill_review','care_due']::TEXT[]);

-- Future accounts use the same family-care option; existing accounts retain
-- their prior choices unless they explicitly enable Care reminders.
ALTER TABLE member_notification_preferences
 ALTER COLUMN enabled_kinds SET DEFAULT ARRAY['event_assigned','event_family','bill_review','care_due']::TEXT[];

-- Authorization for all delivery surfaces is checked at *read/send time*.
-- Revoking guardian access or disabling the care module immediately hides
-- previously queued notifications without deleting their audit history.
CREATE OR REPLACE FUNCTION calden_can_view_care_notification(care_id UUID, recipient UUID)
RETURNS BOOLEAN LANGUAGE SQL STABLE AS $care_acl$
 SELECT EXISTS(
  SELECT 1
  FROM family_care_items i
  JOIN family_care_subjects s ON s.id=i.subject_id
  JOIN users u ON u.id=recipient AND u.active=true AND u.is_child=false
  JOIN app_settings a ON a.key=s.kind||'_enabled' AND a.value='true'
  WHERE i.id=care_id AND i.archived=false AND s.archived=false
   AND (
    (s.kind='school' AND (
       u.role='admin'
       OR (s.child_user_id IS NULL AND s.created_by=u.id)
       OR (s.child_user_id IS NOT NULL AND EXISTS(
         SELECT 1 FROM family_child_guardians g
         WHERE g.child_id=s.child_user_id AND g.guardian_id=u.id))
    ))
    OR (s.kind IN ('maintenance','pets') AND
       (u.role IN ('admin','planner') OR s.created_by=u.id OR i.assigned_to=u.id))
   )
 )
$care_acl$;
