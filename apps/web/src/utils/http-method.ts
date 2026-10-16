/**
 * HTTP 方法统一徽标色（Swagger 语义色）
 * GET=blue / POST=green / PUT=orange / DELETE=red / 其他=grey
 */
export function methodColor(method?: string | unknown): string {
  switch (String(method ?? '').toUpperCase()) {
    case 'GET':
      return 'blue';
    case 'POST':
      return 'green';
    case 'PUT':
      return 'orange';
    case 'DELETE':
      return 'red';
    default:
      return 'grey';
  }
}
