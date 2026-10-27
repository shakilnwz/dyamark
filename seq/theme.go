// Package seq holds the sequence-diagram pieces shared by its parser,
// layout and drawing subpackages.
package seq

import "image/color"

// Theme defines colours and style for sequence diagram elements.
type Theme struct {
	ActorBkg         color.NRGBA
	ActorBorder      color.NRGBA
	ActorText        color.NRGBA
	Lifeline         color.NRGBA
	Signal           color.NRGBA
	SignalText       color.NRGBA
	ActivationBkg    color.NRGBA
	ActivationBorder color.NRGBA
	NoteBkg          color.NRGBA
	NoteBorder       color.NRGBA
	NoteText         color.NRGBA
	FrameBkg         color.NRGBA
	FrameBorder      color.NRGBA
	FrameText        color.NRGBA
	// Rounding is the corner radius in dp. Zero means square corners.
	Rounding float32
}
