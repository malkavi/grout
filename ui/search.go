package ui

import (
	"errors"

	gaba "github.com/UncleJunVIP/gabagool/v2/pkg/gabagool"
)

type SearchInput struct {
	InitialText string
}

type SearchOutput struct {
	Query string
}

type SearchScreen struct{}

func NewSearchScreen() *SearchScreen {
	return &SearchScreen{}
}

func (s *SearchScreen) Draw(input SearchInput) (ScreenResult[SearchOutput], error) {
	res, err := gaba.Keyboard(input.InitialText)
	if err != nil {
		if errors.Is(err, gaba.ErrCancelled) {
			// User cancelled - not an error, just go back
			return Back(SearchOutput{}), nil
		}
		gaba.GetLogger().Error("Error with keyboard", "error", err)
		return WithCode(SearchOutput{}, gaba.ExitCodeError), err
	}

	return Success(SearchOutput{Query: res.Text}), nil
}
