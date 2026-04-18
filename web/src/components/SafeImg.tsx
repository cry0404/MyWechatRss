import { cn } from "@/lib/cn";

/** WeChat MP/CDN images often block requests with a Referer from other origins; no-referrer fixes hotlink placeholders. */
export function SafeImg({ className, ...props }: React.ImgHTMLAttributes<HTMLImageElement>) {
  return <img {...props} className={cn(className)} referrerPolicy="no-referrer" />;
}
