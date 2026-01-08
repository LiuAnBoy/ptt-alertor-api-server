package models

import (
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

var User = func() *user.User {
	return user.NewUser(new(user.Redis))
}

var Article = func() *article.Article {
	return article.NewArticle(new(article.Postgres))
}

var Board = func() *board.Board {
	return board.NewBoard(new(board.Postgres), new(board.Redis))
}
