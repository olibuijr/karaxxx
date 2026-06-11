export function parseCategories(value: string | null | undefined): string[] {
  if (!value) return []

  const seen = new Set<string>()
  const categories: string[] = []

  for (const rawCategory of value.split(',')) {
    const category = rawCategory.trim()
    if (!category || seen.has(category)) continue
    seen.add(category)
    categories.push(category)
  }

  return categories
}

export function toggleCategory(categories: readonly string[], category: string): string[] {
  const normalizedCategory = category.trim()
  if (!normalizedCategory) return [...categories]

  return categories.includes(normalizedCategory)
    ? categories.filter((item) => item !== normalizedCategory)
    : [...categories, normalizedCategory]
}

export function toggleCategoryParam(
  value: string | null | undefined,
  category: string,
): string | undefined {
  const nextCategories = toggleCategory(parseCategories(value), category)
  return nextCategories.length > 0 ? nextCategories.join(',') : undefined
}
