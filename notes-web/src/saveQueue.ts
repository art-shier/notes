export type SaveState = 'pending' | 'saving' | 'saved' | 'error' | 'conflict';
export type Patch = Record<string, unknown>;
type RecordState = {version:number;pending:Patch;inflight?:Patch;error?:unknown;promise?:Promise<boolean>;timer?:ReturnType<typeof setTimeout>};

// Requests for one note are serialized; newer edits remain pending until acknowledged.
export class SaveQueue {
  private records = new Map<string,RecordState>();
  private send:(id:string,version:number,patch:Patch)=>Promise<any>;
  private status:(id:string,state:SaveState,error?:unknown)=>void;
  private saved:(id:string,result:any)=>void;
  private draft:(id:string,version:number,patch:Patch|null)=>void;
  constructor(send:SaveQueue['send'],status:SaveQueue['status'],saved:SaveQueue['saved'],draft:SaveQueue['draft']) {
    this.send=send;this.status=status;this.saved=saved;this.draft=draft;
  }
  stage(id:string,version:number,patch:Patch) {
    const r=this.records.get(id)||{version,pending:{}};
    r.pending={...r.pending,...patch};this.records.set(id,r);
    this.draft(id,r.version,{...r.inflight,...r.pending});
    if(!r.error)this.status(id,'pending');
    if(r.timer)clearTimeout(r.timer);
    r.timer=setTimeout(()=>{void this.flush(id)},800);
  }
  async flush(id:string,retry=false):Promise<boolean> {
    const r=this.records.get(id);if(!r)return true;
    if(r.timer)clearTimeout(r.timer);
    if(r.promise)return r.promise;
    if(r.error&&!retry)return false;
    r.error=undefined;
    r.promise=(async()=>{
      while(Object.keys(r.pending).length){
        const sent=r.pending;r.pending={};r.inflight=sent;this.status(id,'saving');
        try {
          const result=await this.send(id,r.version,sent);
          r.version=result.version;r.inflight=undefined;this.saved(id,result);
          this.draft(id,r.version,Object.keys(r.pending).length?r.pending:null);
        } catch(error){
          r.pending={...sent,...r.pending};r.inflight=undefined;r.error=error;
          this.draft(id,r.version,r.pending);
          this.status(id,(error as {status?:number}).status===409?'conflict':'error',error);return false;
        }
      }
      this.status(id,'saved');return true;
    })().finally(()=>{r.promise=undefined});
    return r.promise;
  }
  async flushAll(){return (await Promise.all([...this.records.keys()].map(id=>this.flush(id)))).every(Boolean)}
  adopt(id:string,version:number){const r=this.records.get(id);if(r&&!r.promise&&!Object.keys(r.pending).length){r.version=version;r.error=undefined}}
  versionFor(id:string,fallback:number){return this.records.get(id)?.version||fallback}
  discard(id:string){const r=this.records.get(id);if(r?.timer)clearTimeout(r.timer);this.records.delete(id)}
  dispose(){for(const r of this.records.values())if(r.timer)clearTimeout(r.timer)}
  hasPending(){return [...this.records.values()].some(r=>r.promise||Object.keys(r.pending).length)}
}
