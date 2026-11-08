export function versionLabel(value:string):string {
 return /^v?(\d+\.\d+(?:\.\d+)?)(?:[-+].*)?$/.exec(value)?.[1]??value;
}
export function releaseLabel(value:string,versions:string[]):string {
 const base=versionLabel(value);
 if(versions.filter(v=>versionLabel(v)===base).length<2)return base;
 const suffix=/-([a-z]+\d+)$/i.exec(value)?.[1];
 return suffix?`${base} · ${suffix.toUpperCase()}`:value;
}
