package top

import (
	"fmt"
)

var statsRepo = &Postgres{}

// ListKeywords returns top keywords as "board:keyword" strings
func ListKeywords(num int) []string {
	return listByType("keyword", num)
}

// ListAuthors returns top authors as "board:author" strings
func ListAuthors(num int) []string {
	return listByType("author", num)
}

// ListPushSum returns top pushsum as "board:value" strings
func ListPushSum(num int) []string {
	return listByType("pushsum", num)
}

// listByType fetches stats from PostgreSQL and formats as "board:value"
func listByType(subType string, num int) []string {
	stats, err := statsRepo.ListByType(subType, num)
	if err != nil {
		return nil
	}

	result := make([]string, 0, len(stats))
	for _, stat := range stats {
		result = append(result, fmt.Sprintf("%s:%s", stat.Board, stat.Value))
	}
	return result
}

// ListTopFormatted returns formatted top rankings for Telegram display
func ListTopFormatted(num int) string {
	content := "關鍵字"
	for i, keyword := range ListKeywords(num) {
		content += fmt.Sprintf("\n%d. %s", i+1, keyword)
	}

	content += "\n----\n作者"
	for i, author := range ListAuthors(num) {
		content += fmt.Sprintf("\n%d. %s", i+1, author)
	}

	content += "\n----\n推噓文"
	for i, pushSum := range ListPushSum(num) {
		content += fmt.Sprintf("\n%d. %s", i+1, pushSum)
	}

	content += "\n\nTOP 100:\nhttps://ptt.luan.com.tw/top"
	return content
}
