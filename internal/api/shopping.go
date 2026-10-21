package api

import (
    "encoding/json"
    "errors"
    "fmt"
    "math"
    "net/http"
    "strings"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
)

// Shopping lists can be household-shared or owner-private. Private lists do
// not become administrator-readable merely because a member is an admin.
type shoppingListInput struct {
    Name string `json:"name"`
    Shared bool `json:"shared"`
    Archived bool `json:"archived"`
}
type shoppingItemInput struct {
    Name string `json:"name"`
    Quantity float64 `json:"quantity"`
    Unit string `json:"unit"`
    Notes string `json:"notes"`
    Purchased bool `json:"purchased"`
    Category string `json:"category"`
    AssignedTo *uuid.UUID `json:"assigned_to"`
    StockOnHand *float64 `json:"stock_on_hand"`
    ReorderAt *float64 `json:"reorder_at"`
}
var shoppingUnits = map[string]bool{
    "each":true,"pack":true,"box":true,"bag":true,"lb":true,
    "oz":true,"kg":true,"g":true,"L":true,"mL":true,
}
func normalizeShoppingList(in shoppingListInput) (shoppingListInput,bool) {
    in.Name=strings.TrimSpace(in.Name)
    return in,len(in.Name)>0&&len(in.Name)<=120
}
func normalizeShoppingItem(in shoppingItemInput) (shoppingItemInput,bool) {
    in.Name=strings.TrimSpace(in.Name)
    in.Notes=strings.TrimSpace(in.Notes)
    in.Category=strings.TrimSpace(in.Category)
    if in.Unit=="" {in.Unit="each"}
    if len(in.Name)==0||len(in.Name)>200||len(in.Notes)>2000||len(in.Category)>80 {return in,false}
    if in.AssignedTo!=nil&&*in.AssignedTo==uuid.Nil{return in,false}
    if in.StockOnHand!=nil&&(!shoppingFinite(*in.StockOnHand)||*in.StockOnHand<0||*in.StockOnHand>1000000){return in,false}
    if in.ReorderAt!=nil&&(!shoppingFinite(*in.ReorderAt)||*in.ReorderAt<=0||*in.ReorderAt>1000000){return in,false}
    if !shoppingFinite(in.Quantity)||in.Quantity<=0||in.Quantity>1000000||!shoppingUnits[in.Unit] {return in,false}
    return in,true
}
func shoppingFinite(v float64)bool{return !math.IsNaN(v)&&!math.IsInf(v,0)}
// Decode old item payloads without losing metadata. PUT only replaces new
// metadata fields when their JSON key is explicitly present, including null.
func decodeShoppingInput(r *http.Request)(shoppingItemInput,map[string]bool,error){
    var fields map[string]json.RawMessage
    if err:=decode(r,&fields);err!=nil{return shoppingItemInput{},nil,err}
    if fields==nil{return shoppingItemInput{},nil,fmt.Errorf("empty shopping item")}
    allowed:=map[string]bool{
      "name":true,"quantity":true,"unit":true,"notes":true,"purchased":true,
      "category":true,"assigned_to":true,"stock_on_hand":true,"reorder_at":true,
    }
    for key:=range fields{
        if !allowed[key]{return shoppingItemInput{},nil,fmt.Errorf("unrecognized shopping field")}
    }
    raw,err:=json.Marshal(fields)
    if err!=nil{return shoppingItemInput{},nil,err}
    var in shoppingItemInput
    if err=json.Unmarshal(raw,&in);err!=nil{return shoppingItemInput{},nil,err}
    present:=map[string]bool{}
    for _,key:=range []string{"category","assigned_to","stock_on_hand","reorder_at"}{
        _,present[key]=fields[key]
    }
    return in,present,nil
}
func (s *server) allowedShoppingAssignee(r *http.Request,listID uuid.UUID,person *uuid.UUID)bool{
    if person==nil{return true}
    var ok bool
    err:=s.db.QueryRow(r.Context(),`SELECT EXISTS(
      SELECT 1 FROM users u JOIN household_shopping_lists l ON l.id=$1
      WHERE u.id=$2 AND u.active=true
      AND (l.shared=true OR l.created_by=u.id)
      AND (u.is_child=false OR EXISTS(
        SELECT 1 FROM family_child_profiles p WHERE p.user_id=u.id
        AND 'shopping'=ANY(p.allowed_modules)))
    )`,listID,*person).Scan(&ok)
    return err==nil&&ok
}
func shoppingActorCanEdit(r *http.Request) bool {
    return currentActor(r).Role!="restricted"
}
func (s *server) requireShopping(w http.ResponseWriter,r *http.Request) bool {
    var enabled bool
    err:=s.db.QueryRow(r.Context(),
        "SELECT COALESCE((SELECT value!='false' FROM app_settings WHERE key='shopping_enabled'),true)").Scan(&enabled)
    if err!=nil {writeError(w,500,"Could not check shopping feature settings");return false}
    if !enabled {writeError(w,403,"Shopping is turned off by your household administrator");return false}
    return true
}
// Only list items are locked when a list is archived. Its owner may still
// open settings to unarchive it without deleting any entries.
func (s *server) requireWritableShoppingList(w http.ResponseWriter,r *http.Request,id uuid.UUID) bool {
    var archived bool
    err:=s.db.QueryRow(r.Context(),"SELECT archived FROM household_shopping_lists WHERE id=$1",id).Scan(&archived)
    if err!=nil {writeError(w,404,"Shopping list not found");return false}
    if archived {writeError(w,409,"Unarchive this list before making changes");return false}
    return true
}
func (s *server) shoppingListAccess(w http.ResponseWriter,r *http.Request,modify bool) (uuid.UUID,bool) {
    if !s.requireShopping(w,r) {return uuid.Nil,false}
    id,err:=uuid.Parse(r.PathValue("id"))
    if err!=nil {writeError(w,400,"Invalid shopping list");return uuid.Nil,false}
    if modify&&!shoppingActorCanEdit(r) {writeError(w,403,"You cannot change shopping lists");return uuid.Nil,false}
    var allowed bool
    err=s.db.QueryRow(r.Context(),
      `SELECT EXISTS(SELECT 1 FROM household_shopping_lists
         WHERE id=$1 AND (created_by=$2 OR shared=true))`,
      id,currentActor(r).ID).Scan(&allowed)
    if err!=nil {writeError(w,500,"Could not check shopping list access");return uuid.Nil,false}
    if !allowed {writeError(w,404,"Shopping list not found");return uuid.Nil,false}
    return id,true
}
func (s *server) listShoppingLists(w http.ResponseWriter,r *http.Request) {
    if !s.requireShopping(w,r){return}
    rows,err:=s.db.Query(r.Context(),`
       SELECT l.id,l.name,l.shared,l.archived,l.created_by,l.updated_at,
          (SELECT count(*) FROM household_shopping_items i WHERE i.list_id=l.id) AS total,
          (SELECT count(*) FROM household_shopping_items i WHERE i.list_id=l.id AND i.purchased=false) AS remaining,
          (SELECT count(*) FROM household_shopping_items i WHERE i.list_id=l.id
            AND i.stock_on_hand IS NOT NULL AND i.reorder_at IS NOT NULL
            AND i.stock_on_hand<=i.reorder_at) AS low_stock
       FROM household_shopping_lists l
       WHERE l.created_by=$1 OR l.shared=true
       ORDER BY l.archived,l.updated_at DESC,l.id`,currentActor(r).ID)
    if err!=nil {writeError(w,500,"Could not load shopping lists");return}
    defer rows.Close()
    out:=[]map[string]any{}
    for rows.Next() {
        var id,owner uuid.UUID
        var name string
        var shared,archived bool
        var updatedAt any
        var total,remaining,lowStock int64
        if err=rows.Scan(&id,&name,&shared,&archived,&owner,&updatedAt,&total,&remaining,&lowStock);err!=nil{break}
        out=append(out,map[string]any{"id":id,"name":name,"shared":shared,"archived":archived,
          "created_by":owner,"updated_at":updatedAt,"total":total,"remaining":remaining,
          "low_stock":lowStock,
          "can_manage":owner==currentActor(r).ID,"can_edit":shoppingActorCanEdit(r)})
    }
    if err==nil {err=rows.Err()}
    if err!=nil {writeError(w,500,"Could not read shopping lists");return}
    writeJSON(w,200,map[string]any{"items":out})
}
func (s *server) createShoppingList(w http.ResponseWriter,r *http.Request) {
    if !s.requireShopping(w,r){return}
    if !shoppingActorCanEdit(r){writeError(w,403,"You cannot create shopping lists");return}
    var in shoppingListInput
    if decode(r,&in)!=nil {writeError(w,400,"Check the list details");return}
    var ok bool
    if in,ok=normalizeShoppingList(in);!ok {writeError(w,400,"List name must be 1–120 characters");return}
    var id uuid.UUID
    err:=s.db.QueryRow(r.Context(),`INSERT INTO household_shopping_lists(name,shared,archived,created_by)
      VALUES($1,$2,false,$3) RETURNING id`,in.Name,in.Shared,currentActor(r).ID).Scan(&id)
    if err!=nil {writeError(w,500,"Could not create shopping list");return}
    s.audit(r,"create","shopping_list",&id,"Created shopping list",nil)
    writeJSON(w,201,map[string]any{"id":id})
}
func (s *server) saveShoppingList(w http.ResponseWriter,r *http.Request) {
    id,ok:=s.shoppingListAccess(w,r,true);if !ok{return}
    var in shoppingListInput
    if decode(r,&in)!=nil {writeError(w,400,"Check the list details");return}
    if in,ok=normalizeShoppingList(in);!ok {writeError(w,400,"List name must be 1–120 characters");return}
    tag,err:=s.db.Exec(r.Context(),`UPDATE household_shopping_lists
      SET name=$3,shared=$4,archived=$5,updated_at=now()
      WHERE id=$1 AND created_by=$2`,id,currentActor(r).ID,in.Name,in.Shared,in.Archived)
    if err!=nil {writeError(w,500,"Could not save shopping list");return}
    if tag.RowsAffected()==0 {writeError(w,403,"Only the list owner can change its settings");return}
    s.audit(r,"update","shopping_list",&id,"Updated shopping list",nil)
    writeJSON(w,200,map[string]any{"id":id})
}
func (s *server) deleteShoppingList(w http.ResponseWriter,r *http.Request) {
    id,ok:=s.shoppingListAccess(w,r,true);if !ok{return}
    tag,err:=s.db.Exec(r.Context(),`DELETE FROM household_shopping_lists WHERE id=$1 AND created_by=$2`,id,currentActor(r).ID)
    if err!=nil {writeError(w,500,"Could not delete shopping list");return}
    if tag.RowsAffected()==0 {writeError(w,403,"Only the list owner can delete it");return}
    s.audit(r,"delete","shopping_list",&id,"Deleted shopping list",nil)
    w.WriteHeader(http.StatusNoContent)
}
func (s *server) listShoppingItems(w http.ResponseWriter,r *http.Request) {
    id,ok:=s.shoppingListAccess(w,r,false);if !ok{return}
    rows,err:=s.db.Query(r.Context(),`SELECT i.id,i.name,i.quantity::float8,i.unit,i.notes,i.purchased,
       i.purchased_at,i.purchased_by,i.created_by,i.updated_at,
       i.category,i.assigned_to,i.stock_on_hand::float8,i.reorder_at::float8
       FROM household_shopping_items i WHERE i.list_id=$1
         AND EXISTS(SELECT 1 FROM household_shopping_lists l WHERE l.id=$1
           AND (l.created_by=$2 OR l.shared=true))
       ORDER BY i.purchased,i.created_at,i.id`,id,currentActor(r).ID)
    if err!=nil {writeError(w,500,"Could not load shopping items");return}
    defer rows.Close()
    out:=[]map[string]any{}
    for rows.Next() {
        var itemID,creator uuid.UUID
        var name,unit,notes string
        var quantity float64
        var purchased bool
        var purchasedAt any
        var purchasedBy *uuid.UUID
        var updatedAt any
        var category string
        var assignedTo *uuid.UUID
        var stockOnHand,reorderAt *float64
        if err=rows.Scan(&itemID,&name,&quantity,&unit,&notes,&purchased,&purchasedAt,&purchasedBy,&creator,&updatedAt,&category,&assignedTo,&stockOnHand,&reorderAt);err!=nil{break}
        lowStock:=stockOnHand!=nil&&reorderAt!=nil&&*stockOnHand<=*reorderAt
        out=append(out,map[string]any{"id":itemID,"name":name,"quantity":quantity,
            "unit":unit,"notes":notes,"purchased":purchased,"purchased_at":purchasedAt,
            "purchased_by":purchasedBy,"created_by":creator,"updated_at":updatedAt,
            "category":category,"assigned_to":assignedTo,"stock_on_hand":stockOnHand,
            "reorder_at":reorderAt,"low_stock":lowStock})
    }
    if err==nil {err=rows.Err()}
    if err!=nil {writeError(w,500,"Could not read shopping items");return}
    writeJSON(w,200,map[string]any{"items":out})
}
func (s *server) createShoppingItem(w http.ResponseWriter,r *http.Request) {
    id,ok:=s.shoppingListAccess(w,r,true);if !ok{return}
    if !s.requireWritableShoppingList(w,r,id){return}
    in,_,err:=decodeShoppingInput(r)
    if err!=nil {writeError(w,400,"Check the item details");return}
    if in,ok=normalizeShoppingItem(in);!ok {writeError(w,400,"Choose valid item details");return}
    if in.ReorderAt!=nil&&in.StockOnHand==nil {writeError(w,400,"Enter stock on hand with a reorder level");return}
    if !s.allowedShoppingAssignee(r,id,in.AssignedTo){writeError(w,400,"Choose a member who can use this list");return}
    var itemID uuid.UUID
    err=s.db.QueryRow(r.Context(),`INSERT INTO household_shopping_items
       (list_id,name,quantity,unit,notes,purchased,purchased_at,purchased_by,created_by,
        category,assigned_to,stock_on_hand,reorder_at,last_edited_by)
       SELECT $1,$2,$3,$4,$5,$6,CASE WHEN $6 THEN now() ELSE NULL END,
         CASE WHEN $6 THEN $7::uuid ELSE NULL::uuid END,$7,$8,$9,$10,$11,$7
       FROM household_shopping_lists l WHERE l.id=$1 AND l.archived=false
         AND (l.created_by=$7 OR l.shared=true) RETURNING id`,
       id,in.Name,in.Quantity,in.Unit,in.Notes,in.Purchased,currentActor(r).ID,
       in.Category,in.AssignedTo,in.StockOnHand,in.ReorderAt).Scan(&itemID)
    if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"Shopping list no longer available");return}
    if err!=nil {writeError(w,500,"Could not add shopping item");return}
    s.audit(r,"create","shopping_item",&itemID,"Added shopping item",nil)
    writeJSON(w,201,map[string]any{"id":itemID})
}
func (s *server) saveShoppingItem(w http.ResponseWriter,r *http.Request) {
    id,ok:=s.shoppingListAccess(w,r,true);if !ok{return}
    if !s.requireWritableShoppingList(w,r,id){return}
    itemID,err:=uuid.Parse(r.PathValue("item_id"))
    if err!=nil {writeError(w,400,"Invalid shopping item");return}
    in,present,err:=decodeShoppingInput(r)
    if err!=nil {writeError(w,400,"Check the item details");return}
    if in,ok=normalizeShoppingItem(in);!ok {writeError(w,400,"Choose valid item details");return}
    if present["reorder_at"]&&in.ReorderAt!=nil&&
      (!present["stock_on_hand"]||in.StockOnHand==nil){
      writeError(w,400,"Enter current stock when setting a reorder level");return
    }
    if present["assigned_to"]&&!s.allowedShoppingAssignee(r,id,in.AssignedTo){
       writeError(w,400,"Choose a member who can use this list");return
    }
    tag,err:=s.db.Exec(r.Context(),`UPDATE household_shopping_items
      SET name=$3,quantity=$4,unit=$5,notes=$6,purchased=$7,
       purchased_at=CASE WHEN $7 AND NOT purchased THEN now() WHEN NOT $7 THEN NULL ELSE purchased_at END,
       purchased_by=CASE WHEN $7 AND NOT purchased THEN $8::uuid WHEN NOT $7 THEN NULL::uuid ELSE purchased_by END,
       category=CASE WHEN $9 THEN $10 ELSE category END,
       assigned_to=CASE WHEN $11 THEN $12::uuid ELSE assigned_to END,
       stock_on_hand=CASE WHEN $13 THEN $14::numeric ELSE stock_on_hand END,
       reorder_at=CASE WHEN $15 THEN $16::numeric
         WHEN $13 AND $14::numeric IS NULL THEN NULL ELSE reorder_at END,
       last_edited_by=$8,updated_at=now()
       WHERE list_id=$1 AND id=$2
         AND EXISTS(SELECT 1 FROM household_shopping_lists l WHERE l.id=$1
           AND l.archived=false AND (l.created_by=$8 OR l.shared=true))`,
       id,itemID,in.Name,in.Quantity,in.Unit,in.Notes,in.Purchased,currentActor(r).ID,
       present["category"],in.Category,present["assigned_to"],in.AssignedTo,
       present["stock_on_hand"],in.StockOnHand,present["reorder_at"],in.ReorderAt)
    if err!=nil {writeError(w,500,"Could not save shopping item");return}
    if tag.RowsAffected()==0 {writeError(w,404,"Shopping item not found");return}
    s.audit(r,"update","shopping_item",&itemID,"Updated shopping item",nil)
    writeJSON(w,200,map[string]any{"id":itemID})
}
func (s *server) deleteShoppingItem(w http.ResponseWriter,r *http.Request) {
    id,ok:=s.shoppingListAccess(w,r,true);if !ok{return}
    if !s.requireWritableShoppingList(w,r,id){return}
    itemID,err:=uuid.Parse(r.PathValue("item_id"))
    if err!=nil {writeError(w,400,"Invalid shopping item");return}
    // A single statement deletes the item and captures its final snapshot.
    // List deletion cascades never invoke an item-delete trigger.
    var removed uuid.UUID
    err=s.db.QueryRow(r.Context(),`WITH deleted AS (
      DELETE FROM household_shopping_items
      WHERE list_id=$1 AND id=$2 AND EXISTS(
        SELECT 1 FROM household_shopping_lists l WHERE l.id=$1
          AND l.archived=false AND (l.created_by=$3 OR l.shared=true))
      RETURNING id,list_id,name,quantity,unit,category,assigned_to,purchased,stock_on_hand,reorder_at
    ), history AS (
      INSERT INTO household_shopping_history(list_id,item_id,action,item_snapshot,performed_by)
      SELECT list_id,id,'removed',
        jsonb_build_object('name',name,'quantity',quantity,'unit',unit,
          'category',category,'assigned_to',assigned_to,'purchased',purchased,
          'stock_on_hand',stock_on_hand,'reorder_at',reorder_at),$3
      FROM deleted RETURNING item_id
    ) SELECT item_id FROM history`,id,itemID,currentActor(r).ID).Scan(&removed)
    if errors.Is(err,pgx.ErrNoRows) {writeError(w,404,"Shopping item not found");return}
    if err!=nil {writeError(w,500,"Could not delete shopping item");return}
    s.audit(r,"delete","shopping_item",&itemID,"Deleted shopping item",nil)
    w.WriteHeader(http.StatusNoContent)
}


func (s *server) listShoppingHistory(w http.ResponseWriter,r *http.Request){
    id,ok:=s.shoppingListAccess(w,r,false);if !ok{return}
    rows,err:=s.db.Query(r.Context(),`SELECT h.item_id,h.action,h.item_snapshot,
      h.performed_by,h.created_at
      FROM household_shopping_history h WHERE h.list_id=$1
        AND EXISTS(SELECT 1 FROM household_shopping_lists l WHERE l.id=$1
          AND (l.created_by=$2 OR l.shared=true))
      ORDER BY h.created_at DESC,h.id DESC LIMIT 100`,id,currentActor(r).ID)
    if err!=nil{writeError(w,500,"Could not load shopping history");return}
    defer rows.Close()
    out:=[]map[string]any{}
    for rows.Next(){
        var itemID uuid.UUID
        var action string
        var snapshot json.RawMessage
        var actor *uuid.UUID
        var when any
        if err=rows.Scan(&itemID,&action,&snapshot,&actor,&when);err!=nil{break}
        out=append(out,map[string]any{
          "item_id":itemID,"action":action,"item":snapshot,
          "performed_by":actor,"created_at":when,
        })
    }
    if err==nil{err=rows.Err()}
    if err!=nil{writeError(w,500,"Could not read shopping history");return}
    writeJSON(w,200,map[string]any{"items":out})
}
