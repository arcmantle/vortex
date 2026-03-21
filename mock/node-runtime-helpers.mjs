export function slug(value) {
  return String(value).toLowerCase().replace(/\s+/g, "-")
}

export async function describeFile(filePath) {
  return {
    path: filePath,
    note: "mock helper import ok"
  }
}