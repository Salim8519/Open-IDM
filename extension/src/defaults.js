// Default settings shared by the background worker, popup and options page.

export const HOST_NAME = 'com.peregrine.host';

// File types captured regardless of size (lower case, no dot).
export const DEFAULT_EXTENSIONS = [
  // archives
  'zip', 'rar', '7z', 'tar', 'gz', 'tgz', 'bz2', 'xz', 'zst', 'cab',
  // installers / packages / disk images
  'exe', 'msi', 'msix', 'appx', 'dmg', 'pkg', 'iso', 'img', 'apk', 'deb', 'rpm', 'appimage',
  // video
  'mp4', 'mkv', 'avi', 'mov', 'wmv', 'flv', 'webm', 'm4v', 'mpg', 'mpeg', '3gp',
  // audio
  'mp3', 'flac', 'wav', 'aac', 'm4a', 'ogg', 'opus', 'wma',
  // documents
  'pdf', 'epub',
  // other large binaries
  'bin', 'torrent',
];

export const DEFAULT_MIN_SIZE = 1024 * 1024; // used until the app reports minExtSize

export const DEFAULT_SETTINGS = Object.freeze({
  enabled: true,
  minSizeMB: null, // null = use the app's minExtSize
  extensions: DEFAULT_EXTENSIONS,
  excludedSites: [],
});
