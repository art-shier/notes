import type {Patch} from './saveQueue';
type StorageLike=Pick<Storage,'getItem'|'setItem'|'removeItem'|'key'|'length'>;
export interface Draft {version:number;patch:Patch;key:string;raw:string;updatedAt:number}
// Each mounted editor owns its key. Acknowledging a save clears only its draft.
export class DraftStore {
  private userId:string;private storage:StorageLike;private writerId:string;
  private recovered=new Map<string,Draft>();
  constructor(userId:string,storage:StorageLike,writerId:string){this.userId=userId;this.storage=storage;this.writerId=writerId}
  private prefix(id:string){return `shiji.draft.v1.${this.userId}.${id}`}
  private own(id:string){return this.prefix(id)+'.'+this.writerId}
  private clearRecovered(id:string,draft?:Draft){const source=draft||this.recovered.get(id);if(source&&this.storage.getItem(source.key)===source.raw)this.storage.removeItem(source.key);this.recovered.delete(id)}
  write(id:string,version:number,patch:Patch|null){
    if(patch)this.storage.setItem(this.own(id),JSON.stringify({version,patch,updatedAt:Date.now()}));
    else{this.storage.removeItem(this.own(id));this.clearRecovered(id)}
  }
  newest(id:string):Draft|null{
    const candidates:Draft[]=[];const prefix=this.prefix(id);
    for(let i=0;i<this.storage.length;i++){
      const key=this.storage.key(i);if(!key||(key!==prefix&&!key.startsWith(prefix+'.')))continue;
      try{const raw=this.storage.getItem(key)!;const data=JSON.parse(raw);if(Number.isInteger(data.version)&&data.version>0&&data.patch&&typeof data.patch==='object'&&!Array.isArray(data.patch))candidates.push({...data,key,raw,updatedAt:data.updatedAt||0})}catch{}
    }
    return candidates.sort((a,b)=>b.updatedAt-a.updatedAt)[0]||null;
  }
  recover(id:string,draft:Draft){this.recovered.set(id,draft)}
  discard(id:string,draft?:Draft){this.storage.removeItem(this.own(id));this.clearRecovered(id,draft)}
}
