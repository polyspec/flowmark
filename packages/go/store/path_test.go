package store

// docPath is the corpus path of the document of a document table.
func docPath(table string) string { return table + fileSuffix }

// filePath is the corpus path of the file of a row of a files table.
func filePath(table, key string) string { return table + "/" + key + fileSuffix }
