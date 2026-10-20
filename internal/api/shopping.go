package api

import (
    "net/http"
    "strings"

    "github.com/google/uuid"
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
    if in.Unit=="" {in.Unit="each"}
    if len(in.Name)==0||len(in.Name)>200||len(in.Notes)>2000 {return in,false}
    if in.Quantity<=0||in.Quantity>1000000||!shoppingUnits[in.Unit] {return in,false}
    return in,true
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
          (SELECT count(*) FROM household_shopping_items i WHERE i.list_id=l.id AND i.purchased=false) AS remaining
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
        var total,remaining int64
        if err=rows.Scan(&id,&name,&shared,&archived,&owner,&updatedAt,&total,&remaining);err!=nil{break}
        out=append(out,map[string]any{"id":id,"name":name,"shared":shared,"archived":archived,
          "created_by":owner,"updated_at":updatedAt,"total":total,"remaining":remaining,
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
       i.purchased_at,i.purchased_by,i.created_by,i.updated_at
       FROM household_shopping_items i WHERE i.list_id=$1
       ORDER BY i.purchased,i.created_at,i.id`,id)
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
        if err=rows.Scan(&itemID,&name,&quantity,&unit,&notes,&purchased,&purchasedAt,&purchasedBy,&creator,&updatedAt);err!=nil{break}
        out=append(out,map[string]any{"id":itemID,"name":name,"quantity":quantity,
            "unit":unit,"notes":notes,"purchased":purchased,"purchased_at":purchasedAt,
            "purchased_by":purchasedBy,"created_by":creator,"updated_at":updatedAt})
    }
    if err==nil {err=rows.Err()}
    if err!=nil {writeError(w,500,"Could not read shopping items");return}
    writeJSON(w,200,map[string]any{"items":out})
}
func (s *server) createShoppingItem(w http.ResponseWriter,r *http.Request) {
    id,ok:=s.shoppingListAccess(w,r,true);if !ok{return}
    if !s.requireWritableShoppingList(w,r,id){return}
    var in shoppingItemInput
    if decode(r,&in)!=nil {writeError(w,400,"Check the item details");return}
    if in,ok=normalizeShoppingItem(in);!ok {writeError(w,400,"Choose a name, quantity and valid unit");return}
    var itemID uuid.UUID
    err:=s.db.QueryRow(r.Context(),`INSERT INTO household_shopping_items
       (list_id,name,quantity,unit,notes,purchased,purchased_at,purchased_by,created_by)
       VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $6 THEN now() ELSE NULL END,
         CASE WHEN $6 THEN $7::uuid ELSE NULL::uuid END,$7) RETURNING id`,
       id,in.Name,in.Quantity,in.Unit,in.Notes,in.Purchased,currentActor(r).ID).Scan(&itemID)
    if err!=nil {writeError(w,500,"Could not add shopping item");return}
    s.audit(r,"create","shopping_item",&itemID,"Added shopping item",nil)
    writeJSON(w,201,map[string]any{"id":itemID})
}
func (s *server) saveShoppingItem(w http.ResponseWriter,r *http.Request) {
    id,ok:=s.shoppingListAccess(w,r,true);if !ok{return}
    if !s.requireWritableShoppingList(w,r,id){return}
    itemID,err:=uuid.Parse(r.PathValue("item_id"))
    if err!=nil {writeError(w,400,"Invalid shopping item");return}
    var in shoppingItemInput
    if decode(r,&in)!=nil {writeError(w,400,"Check the item details");return}
    if in,ok=normalizeShoppingItem(in);!ok {writeError(w,400,"Choose a name, quantity and valid unit");return}
    tag,err:=s.db.Exec(r.Context(),`UPDATE household_shopping_items
      SET name=$3,quantity=$4,unit=$5,notes=$6,purchased=$7,
       purchased_at=CASE WHEN $7 AND NOT purchased THEN now() WHEN NOT $7 THEN NULL ELSE purchased_at END,
       purchased_by=CASE WHEN $7 AND NOT purchased THEN $8::uuid WHEN NOT $7 THEN NULL::uuid ELSE purchased_by END,
       updated_at=now() WHERE list_id=$1 AND id=$2`,
       id,itemID,in.Name,in.Quantity,in.Unit,in.Notes,in.Purchased,currentActor(r).ID)
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
    tag,err:=s.db.Exec(r.Context(),`DELETE FROM household_shopping_items WHERE list_id=$1 AND id=$2`,id,itemID)
    if err!=nil {writeError(w,500,"Could not delete shopping item");return}
    if tag.RowsAffected()==0 {writeError(w,404,"Shopping item not found");return}
    s.audit(r,"delete","shopping_item",&itemID,"Deleted shopping item",nil)
    w.WriteHeader(http.StatusNoContent)
}

