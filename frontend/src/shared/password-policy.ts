export function passwordError(value:string):string|undefined {
 if([...value].length<8)return 'Не менее 8 Unicode-символов';
 if(new TextEncoder().encode(value).length>256)return 'Не более 256 байт UTF-8';
 if([...value].some(c=>{const n=c.codePointAt(0)!;return n>=0xd800&&n<=0xdfff})||/[\0\r\n]/.test(value))return 'Используйте корректный Unicode без перевода строки';
 return undefined;
}
