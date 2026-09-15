package data

//go:generate go tool ent generate --feature=intercept,sql/modifier,sql/lock --target ./ent ./schema
