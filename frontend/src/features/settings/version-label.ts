// Display never replaces the immutable ID sent to the API.
export function versionLabel(value:string):string {
 const historical=/^v?(\d+\.\d+\.\d+)-stage\d+(?:-a\d+)?$/.exec(value);
 if(historical)return historical[1];
 return value.replace(/^v(?=\d)/,'').split('+')[0];
}
export function releaseLabel(value:string,_versions:string[]):string {return versionLabel(value)}
export function publishedLabel(value?:string):string {
 if(!value||!Number.isFinite(Date.parse(value)))return 'Дата не указана';
 return new Date(value).toLocaleDateString('ru-RU',{day:'numeric',month:'long',year:'numeric'});
}
