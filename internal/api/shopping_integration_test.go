package api

import (
 "context"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/google/uuid"
 "github.com/gigabytegrove/calden/internal/store"
 "github.com/jackc/pgx/v5/pgxpool"
)

// This test always requires an isolated database. It refuses to touch a live
// Calden database, even when the environment variable points at one.
func TestShoppingSupplyMigrationLifecycle(t *testing.T) {
 raw:=strings.TrimSpace(os.Getenv("CALDEN_TEST_DATABASE_URL"))
 if raw==""{t.Skip("Set CALDEN_TEST_DATABASE_URL for an isolated PostgreSQL test")}
 cfg,err:=pgxpool.ParseConfig(raw)
 if err!=nil{t.Fatalf("Invalid test URL: %v",err)}
 dbName:=cfg.ConnConfig.Database
 if dbName!="calden_test"&&!strings.HasPrefix(dbName,"calden_test_"){
  t.Fatalf("Refusing to migrate non-test database %q",dbName)
 }
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second)
 defer cancel()
 db,err:=store.Open(ctx,raw)
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 if err:=store.Migrate(ctx,db);err!=nil{t.Fatalf("Migration failed: %v",err)}
 tx,err:=db.Begin(ctx)
 if err!=nil{t.Fatal(err)}
 defer func(){_ = tx.Rollback(context.Background())}()

 var owner,other,listID,itemID uuid.UUID
 for _,entry:=range []struct{id *uuid.UUID;username string}{
  {&owner,"shopping_test_"+uuid.NewString()},
  {&other,"shopping_test_"+uuid.NewString()},
 }{
  if err:=tx.QueryRow(ctx,`INSERT INTO users(username,display_name,password_hash,role,initials)
    VALUES($1,'Shopping Test','invalid-test-login','member','ST') RETURNING id`,
    entry.username).Scan(entry.id);err!=nil{t.Fatal(err)}
 }
 if err:=tx.QueryRow(ctx,`INSERT INTO household_shopping_lists(name,shared,created_by)
  VALUES('Private supplies',false,$1) RETURNING id`,owner).Scan(&listID);err!=nil{t.Fatal(err)}
 if err:=tx.QueryRow(ctx,`INSERT INTO household_shopping_items
  (list_id,name,quantity,unit,created_by,category,assigned_to,stock_on_hand,reorder_at,last_edited_by)
  VALUES($1,'Laundry soap',2,'pack',$2,'Cleaning',$2,1,2,$2) RETURNING id`,
  listID,owner).Scan(&itemID);err!=nil{t.Fatal(err)}

 var action,category string
 if err:=tx.QueryRow(ctx,`SELECT action,item_snapshot->>'category'
  FROM household_shopping_history WHERE list_id=$1 AND item_id=$2`,listID,itemID).
  Scan(&action,&category);err!=nil{t.Fatal(err)}
 if action!="added"||category!="Cleaning"{t.Fatalf("Create snapshot=%q %q",action,category)}

 if _,err:=tx.Exec(ctx,`UPDATE household_shopping_items SET purchased=true,
  purchased_by=$2,last_edited_by=$2 WHERE id=$1`,itemID,owner);err!=nil{t.Fatal(err)}
 var purchaseCount int
 if err:=tx.QueryRow(ctx,`SELECT count(*) FROM household_shopping_history
  WHERE item_id=$1 AND action='purchased' AND performed_by=$2`,
  itemID,owner).Scan(&purchaseCount);err!=nil{t.Fatal(err)}
 if purchaseCount!=1{t.Fatalf("Expected one purchased snapshot, got %d",purchaseCount)}

 // A direct delete must preserve a final snapshot without an item FK.
 var removed uuid.UUID
 err=tx.QueryRow(ctx,`WITH deleted AS (
   DELETE FROM household_shopping_items WHERE id=$1 AND list_id=$2
   RETURNING id,list_id,name,quantity,unit,category,assigned_to,purchased,stock_on_hand,reorder_at
 ), history AS (
   INSERT INTO household_shopping_history(list_id,item_id,action,item_snapshot,performed_by)
   SELECT list_id,id,'removed',jsonb_build_object(
    'name',name,'quantity',quantity,'unit',unit,'category',category,
    'assigned_to',assigned_to,'purchased',purchased,'stock_on_hand',stock_on_hand,
    'reorder_at',reorder_at),$3 FROM deleted
   RETURNING item_id
 ) SELECT item_id FROM history`,itemID,listID,owner).Scan(&removed)
 if err!=nil||removed!=itemID{t.Fatalf("Delete snapshot failed: %v",err)}
 var historyCount int
 if err:=tx.QueryRow(ctx,`SELECT count(*) FROM household_shopping_history
  WHERE list_id=$1`,listID).Scan(&historyCount);err!=nil{t.Fatal(err)}
 if historyCount!=3{t.Fatalf("Expected three snapshots, got %d",historyCount)}

 // The old AFTER DELETE trigger would reinsert records when cascading
 // through a list deletion. Ensure parent/children/history all disappear.
 if _,err:=tx.Exec(ctx,`INSERT INTO household_shopping_items
  (list_id,name,quantity,unit,created_by) VALUES($1,'Extra',1,'each',$2)`,
  listID,other);err!=nil{t.Fatal(err)}
 if _,err:=tx.Exec(ctx,`DELETE FROM household_shopping_lists WHERE id=$1`,listID);err!=nil{
  t.Fatalf("List cascade failed: %v",err)
 }
 if err:=tx.QueryRow(ctx,`SELECT count(*) FROM household_shopping_history
   WHERE list_id=$1`,listID).Scan(&historyCount);err!=nil{t.Fatal(err)}
 if historyCount!=0{t.Fatalf("Orphaned history after list deletion: %d",historyCount)}
}
