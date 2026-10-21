-- Shopping and supplies: additive, non-destructive metadata and bounded history.
-- Existing Android and older web clients do not need to send these fields.
ALTER TABLE household_shopping_items
 ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT '',
 ADD COLUMN IF NOT EXISTS assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
 ADD COLUMN IF NOT EXISTS stock_on_hand NUMERIC(12,3),
 ADD COLUMN IF NOT EXISTS reorder_at NUMERIC(12,3),
 ADD COLUMN IF NOT EXISTS last_edited_by UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE household_shopping_items
 ADD CONSTRAINT shopping_category_limit CHECK (char_length(category)<=80),
 ADD CONSTRAINT shopping_stock_nonnegative CHECK (stock_on_hand IS NULL OR (stock_on_hand >= 0 AND stock_on_hand <= 1000000)),
 ADD CONSTRAINT shopping_reorder_positive CHECK (reorder_at IS NULL OR (reorder_at > 0 AND reorder_at <= 1000000)),
 ADD CONSTRAINT shopping_reorder_requires_stock CHECK (reorder_at IS NULL OR stock_on_hand IS NOT NULL);
CREATE INDEX IF NOT EXISTS shopping_assignee_idx
 ON household_shopping_items(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE TABLE IF NOT EXISTS household_shopping_history(
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 list_id UUID NOT NULL REFERENCES household_shopping_lists(id) ON DELETE CASCADE,
 item_id UUID NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('added','updated','purchased','needed','removed')),
 item_snapshot JSONB NOT NULL,
 performed_by UUID REFERENCES users(id) ON DELETE SET NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS shopping_history_recent_idx
 ON household_shopping_history(list_id,created_at DESC,id DESC);
CREATE OR REPLACE FUNCTION calden_shopping_item_history() RETURNS TRIGGER
 LANGUAGE plpgsql AS $$
DECLARE record_row household_shopping_items%ROWTYPE;
event_name TEXT;
by_user UUID;
BEGIN
 IF TG_OP='INSERT' THEN
   record_row:=NEW;
   event_name:='added';
   by_user:=NEW.created_by;
 ELSE
   IF NEW IS NOT DISTINCT FROM OLD THEN RETURN NEW; END IF;
   record_row:=NEW;
   event_name:=CASE
     WHEN NEW.purchased IS DISTINCT FROM OLD.purchased AND NEW.purchased THEN 'purchased'
     WHEN NEW.purchased IS DISTINCT FROM OLD.purchased THEN 'needed'
     ELSE 'updated' END;
   by_user:=NEW.last_edited_by;
 END IF;
 INSERT INTO household_shopping_history(list_id,item_id,action,item_snapshot,performed_by)
 VALUES(record_row.list_id,record_row.id,event_name,
        jsonb_build_object('name',record_row.name,'quantity',record_row.quantity,
         'unit',record_row.unit,'category',record_row.category,
         'assigned_to',record_row.assigned_to,'purchased',record_row.purchased,
         'stock_on_hand',record_row.stock_on_hand,'reorder_at',record_row.reorder_at),
        by_user);
 RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS shopping_item_history_trigger ON household_shopping_items;
CREATE TRIGGER shopping_item_history_trigger
 AFTER INSERT OR UPDATE ON household_shopping_items
 FOR EACH ROW EXECUTE FUNCTION calden_shopping_item_history();
