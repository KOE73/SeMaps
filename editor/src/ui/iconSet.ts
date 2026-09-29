/**
 * The Tabler icons the app can show — the ONLY place Tabler names are turned
 * into svg. Everything else asks `ui/icons.ts` (semantic keys) or
 * `ui/kindIcons.ts` (kinds, and any name from this set). The set is curated,
 * not all of Tabler: the icons the app itself uses plus a palette for
 * choosing a project, view or block-style icon (the picker lists exactly
 * these). To offer one more, add its name to the pattern below.
 *
 * The svgs are inlined at build time (Vite `?raw`), `stroke="currentColor"`.
 */
const files = import.meta.glob(
  "/node_modules/@tabler/icons/icons/outline/{alert-triangle,anchor,arrow-back-up,arrow-bar-down,arrow-bar-left,arrow-bar-right,arrow-bar-to-down,arrow-bar-to-left,arrow-bar-to-right,arrow-bar-to-up,arrow-bar-up,arrow-down,arrow-forward-up,arrow-left,arrow-right,arrow-up,arrows-diff,arrows-horizontal,arrows-shuffle,arrows-vertical,binary-tree,books,box,box-model,box-multiple,brackets,building-bank,bulb,category,check,checks,chevron-right,circle-check,circle-dot,circle-x,clipboard,clock,code,compass,copy,cpu,database,device-desktop-code,device-floppy,dots,download,edit,eye,eye-off,file-code,file-export,file-text,filter,focus-2,folder,folder-open,folders,function,ghost,grid-dots,hammer,help-circle,hierarchy,hierarchy-2,home,hourglass,inbox,language,layout-board,layout-list,link,list-check,list-details,list-search,loader-2,lock,magnet,map,math-function,maximize,minus,moon,needle,notes,package,palette,paperclip,pencil,phone-call,pin,player-play,playlist-add,playlist-x,plug,plug-connected,plus,pointer,quote,refresh,replace,restore,robot,route,search,settings,shield-half,sparkles,square-letter-c,square-letter-d,square-letter-e,square-letter-i,square-letter-r,square-letter-s,square-off,square-rounded-letter-r,stack-2,sun,sunglasses,table,trash,wave-sine,world,x,zoom-in,zoom-out,puzzle,brain,cloud,shield,flask,rocket,user,message,chart-bar,building-factory,shopping-cart,credit-card,mail,receipt,list-numbers,tool,file,files,cube,apps,layout-dashboard,terminal-2,api,git-branch,server,network,key,bug,chart-pie,calendar,star,heart,flag,bookmark,tag,bell,camera,code-circle,components,layers-subtract,hexagon,circle,square,triangle,diamond,bolt,flame,leaf,wifi,cloud-upload,upload,adjustments,lock-open,shield-check,certificate,users,user-circle,id,phone,send,message-circle,chart-line,chart-dots,list,arrows-exchange,stack,packages,typography,abc,binary,brackets-contain,hash,variable,versions,template,ticket,tags,cube-plus,forms,app-window,browser,device-desktop,device-mobile,printer,cloud-computing,database-cog,file-settings,arrows-split,target,puzzle-2,sitemap,topology-star-3,corner-right-down,chevron-down}.svg",
  { query: "?raw", import: "default", eager: true },
) as Record<string, string>;

const TABLER = new Map<string, string>();
for (const [path, svg] of Object.entries(files)) TABLER.set(path.replace(/^.*\/(.+)\.svg$/, "$1"), svg);

/** What an unknown name shows. */
export const FALLBACK_ICON = "help-circle";

export function hasTabler(name: string): boolean {
  return TABLER.has(name);
}

/** The svg of a Tabler icon of the set; an unknown name gets the fallback icon. */
export function tablerSvg(name: string | undefined): string {
  return (name && TABLER.get(name)) || TABLER.get(FALLBACK_ICON) || "";
}

/** Every name of the set, sorted — what the icon picker searches. */
export function tablerNames(): string[] {
  return [...TABLER.keys()].sort();
}
