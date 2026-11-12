import { Disc3, File, FileArchive, FileImage, FileMusic, FilePlay, FileText, LayoutGrid, Package } from "lucide-react";

const icons: Record<string, typeof File> = {
  Video: FilePlay,
  Music: FileMusic,
  Documents: FileText,
  Compressed: FileArchive,
  Programs: Package,
  Images: FileImage,
  "Disk Images": Disc3,
  Other: File,
  All: LayoutGrid,
};

export function CategoryIcon({ category, size = 16 }: { category: string; size?: number }) {
  const Icon = icons[category] ?? File;
  return <Icon size={size} aria-hidden="true" className={`cat-icon cat-${category.replace(/\s/g, "")}`} />;
}
