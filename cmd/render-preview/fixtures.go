package main

import "fmt"

type eventCard struct {
	top        int
	heading    string
	count      int64
	start, end string
	ongoing    bool
}

type scene struct {
	date       string
	cards      []eventCard
	hours      map[int]string
	separators []int
	endRow     int
	ending     string
}

// Fixed synthetic rows preserve the reviewed full-screen example, including
// six rows per idle hour at its original size. Narrow previews crop these rows;
// they do not recompute time geometry or exercise production navigation.
func fixture(name string) (scene, error) {
	s := scene{
		date: "September 21, 2026", hours: map[int]string{0: "08:00 AM"},
		cards: []eventCard{
			{top: 6, heading: " Reading · 1h · 09:00 - 10:00 AM", count: 20, start: "09:00 AM"},
			{top: 11, heading: " Walking · 1h · 10:00 - 11:00 AM", count: 0, start: "10:00 AM", end: "11:00 AM"},
			{top: 17, heading: " Planning · 1h · Started at 11:30 AM · ongoing", count: 10, start: "11:30 AM", ongoing: true},
		},
		separators: []int{10}, endRow: 23, ending: "── Now · 12:30 PM ──",
	}
	switch name {
	case "main":
	case "adjacent":
		s.date = "September 20, 2026"
		s.cards[1].end = ""
		s.cards[2] = eventCard{top: 16, heading: " Planning · 1h · 11:00 AM - 12:00 PM", count: 10, start: "11:00 AM", end: "12:00 PM"}
		s.separators = []int{10, 15}
		s.endRow, s.ending = 20, "..."
	case "gapped":
		s.cards[0].end = "10:00 AM"
		s.cards[1] = eventCard{top: 15, heading: " Walking · 1h · 11:00 AM - 12:00 PM", start: "11:00 AM", end: "12:00 PM"}
		s.cards[2] = eventCard{top: 24, heading: " Planning · 1h · 01:00 - 02:00 PM", count: 10, start: "01:00 PM", end: "02:00 PM"}
		s.separators = nil
		s.date, s.endRow, s.ending = "September 20, 2026", 28, "..."
	case "crowded":
		s.date, s.cards, s.separators = "September 20, 2026", nil, nil
		for i, count := range []int64{0, 1, 5, 10, 20, 35, 3, 8} {
			start := fmt.Sprintf("%02d:%02d AM", 8+i/3, (i%3)*20)
			end := fmt.Sprintf("%02d:%02d AM", 8+(i+1)/3, ((i+1)%3)*20)
			s.cards = append(s.cards, eventCard{top: i * 5, heading: fmt.Sprintf(" Session %d · 20m · %s - %s", i+1, start, end), count: count, start: start})
			if i < 7 {
				s.separators = append(s.separators, i*5+4)
			} else {
				s.cards[i].end = end
			}
		}
		s.endRow, s.ending = 39, "..."
	case "empty":
		s.cards, s.separators = nil, nil
		s.hours = map[int]string{0: "08:00 AM", 6: "09:00 AM", 12: "10:00 AM", 18: "11:00 AM", 24: "12:00 PM"}
		s.endRow = 27
	default:
		return scene{}, fmt.Errorf("scenario must be distributions, main, adjacent, gapped, crowded, or empty")
	}
	return s, nil
}
