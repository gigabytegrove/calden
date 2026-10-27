package api

import (
 "context"
 "errors"

 "github.com/google/uuid"
 "github.com/jackc/pgx/v5"
)
type packageImportResult struct{Created,Skipped,Found int}
// storePackageMailRaw is shared by manual imports, token-based forwarding,
// and secure IMAP scans. It never stores full email or trusts messages to
// update an existing shipment's status or date.
func(s *server) storePackageMailRaw(ctx context.Context,owner uuid.UUID,raw string)(packageImportResult,error){
 out:=packageImportResult{}
 subject,found,err:=extractEmailPackages(raw)
 expectedDate:=extractPackageEstimatedDate(raw)
 if err!=nil{return out,err}
 out.Found=len(found)
 if len(found)==0{return out,nil}
 tx,err:=s.db.Begin(ctx)
 if err!=nil{return out,err}
 defer tx.Rollback(ctx)
 for _,p:=range found {
  if !packageTrackingPattern.MatchString(p.TrackingNumber){continue}
  var id uuid.UUID
  e:=tx.QueryRow(ctx,`INSERT INTO package_shipments(owner_id,label,carrier,tracking_number,status,status_source,expected_date)
  SELECT $1,$2,$3,$4,'unknown','email',NULLIF($5,'')::date WHERE
    COALESCE((SELECT value='true' FROM app_settings WHERE key='packages_enabled'),false)
    AND EXISTS(SELECT 1 FROM users WHERE id=$1 AND active AND NOT is_child AND role!='restricted')
  ON CONFLICT DO NOTHING RETURNING id`,owner,subject,p.Carrier,p.TrackingNumber,expectedDate).Scan(&id)
  if errors.Is(e,pgx.ErrNoRows){out.Skipped++;continue}
  if e!=nil{return out,e}
  _,e=tx.Exec(ctx,`INSERT INTO package_checkpoints(shipment_id,status,detail,source)
  VALUES($1,'unknown','Imported from email; status not carrier verified','email')`,id)
  if e!=nil{return out,e}
  out.Created++
 }
 if err=tx.Commit(ctx);err!=nil{return out,err}
 return out,nil
}
