package ui

import (
	"errors"
	"grout/romm"
	"grout/utils"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

type CollectionSelectionInput struct {
	Host                 romm.Host
	LastSelectedIndex    int
	LastSelectedPosition int
}

type CollectionSelectionOutput struct {
	SelectedCollection   romm.Collection
	LastSelectedIndex    int
	LastSelectedPosition int
}

type CollectionSelectionScreen struct{}

func NewCollectionSelectionScreen() *CollectionSelectionScreen {
	return &CollectionSelectionScreen{}
}

func (s *CollectionSelectionScreen) Draw(input CollectionSelectionInput) (ScreenResult[CollectionSelectionOutput], error) {
	output := CollectionSelectionOutput{
		LastSelectedIndex:    input.LastSelectedIndex,
		LastSelectedPosition: input.LastSelectedPosition,
	}

	rc := utils.GetRommClient(input.Host)
	collections, err := rc.GetCollections()
	if err != nil {
		return withCode(output, gaba.ExitCodeError), err
	}

	if len(collections) == 0 {
		return withCode(output, gaba.ExitCode(404)), nil
	}

	var menuItems []gaba.MenuItem
	for _, collection := range collections {
		menuItems = append(menuItems, gaba.MenuItem{
			Text:     collection.Name,
			Selected: false,
			Focused:  false,
			Metadata: collection,
		})
	}

	footerItems := []gaba.FooterHelpItem{
		{ButtonName: "B", HelpText: "Back"},
		{ButtonName: "A", HelpText: "Select"},
	}

	options := gaba.DefaultListOptions("Collections", menuItems)
	options.FooterHelpItems = footerItems
	options.SelectedIndex = input.LastSelectedIndex
	options.VisibleStartIndex = max(0, input.LastSelectedIndex-input.LastSelectedPosition)

	sel, err := gaba.List(options)
	if err != nil {
		if errors.Is(err, gaba.ErrCancelled) {
			return back(output), nil
		}
		return withCode(output, gaba.ExitCodeError), err
	}

	switch sel.Action {
	case gaba.ListActionSelected:
		collection := sel.Items[sel.Selected[0]].Metadata.(romm.Collection)

		output.SelectedCollection = collection
		output.LastSelectedIndex = sel.Selected[0]
		output.LastSelectedPosition = sel.VisiblePosition
		return success(output), nil

	default:
		return withCode(output, gaba.ExitCodeBack), nil
	}
}
