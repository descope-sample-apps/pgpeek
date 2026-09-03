import { format } from "sql-formatter";

export function formatPostgreSQL(sql) {
  return format(sql, { language: "postgresql", keywordCase: "upper" });
}
