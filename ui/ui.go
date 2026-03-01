package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dgageot/delayfm/player"
	"github.com/dgageot/delayfm/radio"
)

const delayStep = time.Second

// Color palette.
var (
	accent    = lipgloss.Color("#EA80FC")
	accentDim = lipgloss.Color("#9C27B0")
	white     = lipgloss.Color("#FFFFFF")
	green     = lipgloss.Color("#69F0AE")
	red       = lipgloss.Color("#FF5252")
	dimText   = lipgloss.Color("#9E9E9E")
	faintText = lipgloss.Color("#757575")
)

// Reusable styles.
var (
	dimStyle    = lipgloss.NewStyle().Foreground(dimText)
	faintStyle  = lipgloss.NewStyle().Foreground(faintText)
	greenStyle  = lipgloss.NewStyle().Foreground(green)
	whiteStyle  = lipgloss.NewStyle().Foreground(white)
	accentStyle = lipgloss.NewStyle().Foreground(accent)
	redStyle    = lipgloss.NewStyle().Foreground(red)
	linkStyle   = lipgloss.NewStyle().Foreground(accent).Underline(true)
)

type (
	searchResultMsg struct{ stations []radio.Station }
	errMsg          struct{ err error }
	playStartedMsg  struct{}
	tickMsg         struct{}
)

type Model struct {
	nameInput      textinput.Model
	countryInput   textinput.Model
	stations       []radio.Station
	cursor         int
	player         *player.Player
	playingStation *radio.Station
	err            error
	searching      bool
	searched       bool
	connecting     bool
	width          int
	height         int
}

func New(p *player.Player) Model {
	s := loadState()

	name := textinput.New()
	name.Placeholder = "search radios..."
	name.Focus()
	name.CharLimit = 64
	name.Prompt = ""
	name.SetWidth(40)
	name.SetValue(s.Search)

	country := textinput.New()
	country.Placeholder = "FR"
	country.CharLimit = 2
	country.Prompt = ""
	country.SetWidth(4)
	country.SetValue(s.Country)

	return Model{
		nameInput:    name,
		countryInput: country,
		player:       p,
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink, m.tick()}

	if m.nameInput.Value() != "" {
		m.searching = true
		cmds = append(cmds, m.searchCmd())
	}

	return tea.Batch(cmds...)
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) inputFocused() bool {
	return m.nameInput.Focused() || m.countryInput.Focused()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, m.tick()

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case searchResultMsg:
		m.stations = msg.stations
		m.cursor = 0
		m.searching = false
		m.searched = true
		m.err = nil
		if len(m.stations) > 0 {
			m.nameInput.Blur()
			m.countryInput.Blur()
		}
		return m, nil

	case playStartedMsg:
		m.connecting = false
		m.err = nil
		return m, nil

	case errMsg:
		m.err = msg.err
		m.searching = false
		m.connecting = false
		return m, nil
	}

	return m.forwardToInputs(msg)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.player.Stop()
		return m, tea.Quit

	case "q":
		if !m.inputFocused() {
			m.player.Stop()
			return m, tea.Quit
		}

	case "tab":
		switch {
		case m.nameInput.Focused():
			m.nameInput.Blur()
			m.countryInput.Focus()
		case m.countryInput.Focused():
			m.countryInput.Blur()
		default:
			m.nameInput.Focus()
		}
		return m, textinput.Blink

	case "enter":
		if m.inputFocused() {
			if m.nameInput.Value() == "" {
				return m, nil
			}
			saveState(state{Search: m.nameInput.Value(), Country: m.countryInput.Value()})
			m.searching = true
			m.stations = nil
			m.err = nil
			return m, m.searchCmd()
		}
		if len(m.stations) > 0 {
			s := m.stations[m.cursor]
			m.connecting = true
			m.playingStation = &s
			m.err = nil
			return m, m.playCmd(s)
		}
		return m, nil

	case "up", "k":
		if !m.inputFocused() && m.cursor > 0 {
			m.cursor--
			return m, nil
		}

	case "down", "j":
		if !m.inputFocused() && m.cursor < len(m.stations)-1 {
			m.cursor++
			return m, nil
		}

	case "s":
		if !m.inputFocused() {
			m.player.Stop()
			m.playingStation = nil
			return m, nil
		}

	case "+", "=", "right":
		if !m.inputFocused() {
			m.player.AdjustDelay(delayStep)
			return m, nil
		}

	case "-", "left":
		if !m.inputFocused() {
			m.player.AdjustDelay(-delayStep)
			return m, nil
		}

	case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if !m.inputFocused() {
			m.player.SetDelay(time.Duration(msg.String()[0]-'0') * time.Second)
			return m, nil
		}
	}

	return m.forwardToInputs(msg)
}

func (m Model) forwardToInputs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	cmds = append(cmds, cmd)
	m.countryInput, cmd = m.countryInput.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m Model) searchCmd() tea.Cmd {
	name := m.nameInput.Value()
	country := m.countryInput.Value()
	return func() tea.Msg {
		stations, err := radio.Search(context.Background(), name, country)
		if err != nil {
			return errMsg{err}
		}
		return searchResultMsg{stations}
	}
}

func (m Model) playCmd(station radio.Station) tea.Cmd {
	return func() tea.Msg {
		if err := m.player.Play(station.URL, station.Name); err != nil {
			return errMsg{err}
		}
		return playStartedMsg{}
	}
}

var appMargin = lipgloss.NewStyle().Margin(1, 2)

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("Initializing...")
	}

	xMargin, yMargin := appMargin.GetFrameSize()
	appWidth := max(0, m.width-xMargin)
	appHeight := max(0, m.height-yMargin)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(white).
		Background(accentDim).
		Padding(0, 1)

	title := renderGradientBar("ᗺ delay.fm", appWidth)
	help := headerStyle.Width(appWidth).Render("↑↓ navigate • enter play • s stop • ←→ delay • 0-9 jump delay • q quit")

	availHeight := max(0, appHeight-lipgloss.Height(title)-lipgloss.Height(help)-2)
	leftWidth := max(20, (appWidth*4)/10)
	rightWidth := max(0, appWidth-leftWidth-2)

	leftCol := m.viewLeftColumn(leftWidth, availHeight)
	rightCol := m.viewNowPlaying(rightWidth, availHeight)

	cols := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", rightCol)
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", cols, "", help)

	v := tea.NewView(appMargin.Render(content))
	v.AltScreen = true
	return v
}

func (m Model) viewLeftColumn(width, availHeight int) string {
	searchBox := m.viewSearchBox(width)
	listHeight := max(0, availHeight-lipgloss.Height(searchBox))
	listBox := m.viewStationList(width, listHeight)
	return lipgloss.JoinVertical(lipgloss.Left, searchBox, listBox)
}

func (m Model) viewSearchBox(width int) string {
	namePromptStyle := dimStyle
	if m.nameInput.Focused() {
		namePromptStyle = accentStyle
	}

	countryPromptStyle := dimStyle
	if m.countryInput.Focused() {
		countryPromptStyle = accentStyle
	}

	content := namePromptStyle.Render("› ") + m.nameInput.View() +
		"\n" + countryPromptStyle.Render("⚑ ") + m.countryInput.View()
	if m.searching {
		content += "\n" + dimStyle.Italic(true).Render("searching...")
	}

	return drawBoxWithTitle(content, "Search", width, 0, m.inputFocused())
}

func (m Model) viewStationList(width, height int) string {
	if height <= 0 {
		return drawBoxWithTitle("", "Stations", width, height, false)
	}

	innerHeight := max(0, height-boxStyle.GetVerticalFrameSize())
	innerWidth := max(0, width-boxStyle.GetHorizontalFrameSize())
	active := !m.inputFocused() && len(m.stations) > 0

	var content string
	if m.searched && len(m.stations) == 0 && m.err == nil && !m.searching {
		content = dimStyle.Render("no stations found")
	} else if len(m.stations) > 0 {
		content = m.renderStationItems(innerWidth, innerHeight)
	}

	return drawBoxWithTitle(padToHeight(content, innerHeight), "Stations", width, height, active)
}

func (m Model) renderStationItems(maxWidth, maxHeight int) string {
	visible, offset := m.visibleStations(maxHeight)

	var b strings.Builder
	for i, s := range visible {
		if i > 0 {
			b.WriteString("\n")
		}

		selected := offset+i == m.cursor

		bulletStyle, nameStyle, bitrateStyle := faintStyle, dimStyle, faintStyle
		bullet := "○ "
		if selected {
			bulletStyle, nameStyle, bitrateStyle = accentStyle, whiteStyle, dimStyle
			bullet = "● "
		}

		bitrateStr := ""
		if s.Bitrate > 0 {
			bitrateStr = fmt.Sprintf(" %dk", s.Bitrate)
		}

		nameWidth := maxWidth - lipgloss.Width(bullet) - lipgloss.Width(bitrateStr)
		name := s.Name
		if lipgloss.Width(name) > nameWidth {
			name = name[:max(0, nameWidth-1)] + "…"
		}

		b.WriteString(bulletStyle.Render(bullet))
		b.WriteString(nameStyle.Render(name))
		b.WriteString(bitrateStyle.Render(bitrateStr))
	}

	if len(m.stations) > len(visible) {
		b.WriteString("\n")
		b.WriteString(faintStyle.Render(fmt.Sprintf("%d more...", len(m.stations)-len(visible))))
	}

	return b.String()
}

func (m Model) viewNowPlaying(width, height int) string {
	innerHeight := max(0, height-boxStyle.GetVerticalFrameSize())

	var content string
	var active bool

	switch {
	case m.err != nil:
		content = redStyle.Render(fmt.Sprintf("Error:\n%v", m.err))
	case m.selectedStation() != nil:
		content = m.renderStationInfo()
		active = m.player.IsPlaying() || m.connecting
	default:
		content = dimStyle.Render("No station selected.")
	}

	return drawBoxWithTitle(padToHeight(content, innerHeight), "Now Playing", width, height, active)
}

// selectedStation returns the station to display in the right panel.
func (m Model) selectedStation() *radio.Station {
	if m.playingStation != nil {
		return m.playingStation
	}
	if len(m.stations) > 0 {
		return &m.stations[m.cursor]
	}
	return nil
}

func (m Model) renderStationInfo() string {
	s := m.selectedStation()

	nameStyle := whiteStyle.Bold(true)
	if m.player.IsPlaying() {
		nameStyle = greenStyle.Bold(true)
	}

	lines := []string{nameStyle.Render(s.Name), ""}

	switch {
	case m.connecting:
		lines = append(lines, accentStyle.Italic(true).Render("Connecting..."), "")
	case m.player.IsPlaying():
		mode := "Live"
		if m.player.Delay() > 0 {
			mode = "Delayed"
		}
		lines = append(lines,
			dimStyle.Render("Status: Playing ("+mode+")"),
			"",
			m.renderDelayBar(),
			m.renderLevelMeter(),
			"",
		)
	}

	for _, field := range []struct{ label, value string }{
		{"Tags", s.Tags},
		{"Countrycode", s.CountryCode},
		{"Country", s.Country},
		{"State", s.State},
		{"Language", s.Language},
		{"Codec", s.Codec},
		{"Bitrate", func() string {
			if s.Bitrate > 0 {
				return fmt.Sprintf("%d kbps", s.Bitrate)
			}
			return ""
		}()},
	} {
		if field.value != "" {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("%-13s", field.label+":"))+faintStyle.Render(field.value))
		}
	}
	if s.Homepage != "" {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("%-13s", "Homepage:"))+linkStyle.Hyperlink(s.Homepage).Render(s.Homepage))
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderDelayBar() string {
	d := m.player.Delay().Truncate(time.Second)
	maxDelay := 30 * time.Second
	barWidth := 20

	filled := int(float64(barWidth) * float64(d) / float64(maxDelay))
	filled = max(0, min(filled, barWidth))

	bar := strings.Repeat("━", filled) + strings.Repeat("─", barWidth-filled)

	label := " live"
	if d > 0 {
		label = fmt.Sprintf(" -%s", d)
	}

	return dimStyle.Render(bar) + greenStyle.Render(label)
}

var vuGradient = lipgloss.Blend1D(20, lipgloss.Color("#69F0AE"), lipgloss.Color("#FF5252"))

func (m Model) renderLevelMeter() string {
	barWidth := len(vuGradient)
	level := m.player.Level()

	filled := int(float64(barWidth) * level)
	filled = max(0, min(filled, barWidth))

	var b strings.Builder
	for i := range barWidth {
		if i < filled {
			b.WriteString(lipgloss.NewStyle().Foreground(vuGradient[i]).Render("█"))
		} else {
			b.WriteString(dimStyle.Render("░"))
		}
	}

	return b.String() + dimStyle.Render(" VU")
}

func (m Model) visibleStations(availableHeight int) ([]radio.Station, int) {
	maxVisible := max(1, availableHeight-1)
	if len(m.stations) <= availableHeight {
		return m.stations, 0
	}

	offset := max(0, m.cursor-maxVisible/2)
	if offset+maxVisible > len(m.stations) {
		offset = len(m.stations) - maxVisible
	}

	return m.stations[offset : offset+maxVisible], offset
}

// padToHeight pads content with newlines to fill the target height.
func padToHeight(content string, targetHeight int) string {
	if targetHeight <= 0 {
		return content
	}
	actual := lipgloss.Height(content)
	if actual < targetHeight {
		content += strings.Repeat("\n", targetHeight-actual)
	}
	return content
}

// renderGradientBar renders text on a full-width gradient background.
func renderGradientBar(text string, width int) string {
	gradientLeft := lipgloss.Color("#7B1FA2")
	gradientRight := lipgloss.Color("#E040FB")

	padded := " " + text + " "
	textWidth := lipgloss.Width(padded)
	if textWidth < width {
		padded += strings.Repeat(" ", width-textWidth)
	}

	colors := lipgloss.Blend1D(width, gradientLeft, gradientRight)

	var b strings.Builder
	i := 0
	for _, r := range padded {
		if i >= width {
			break
		}
		var bg color.Color = lipgloss.NoColor{}
		if i < len(colors) {
			bg = colors[i]
		}
		b.WriteString(lipgloss.NewStyle().
			Bold(true).
			Foreground(white).
			Background(bg).
			Render(string(r)))
		i += lipgloss.Width(string(r))
	}

	return b.String()
}

var boxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(1, 2)

// drawBoxWithTitle renders a box with a title in the top border.
func drawBoxWithTitle(content, title string, width, height int, active bool) string {
	borderColor := dimText
	if active {
		borderColor = accent
	}

	contentStyle := boxStyle.
		BorderForeground(borderColor).
		Width(max(0, width))

	if height > 0 {
		contentStyle = contentStyle.Height(height).MaxHeight(height)
	}

	rendered := contentStyle.Render(content)

	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}

	titleStr := " " + title + " "

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleRendered := whiteStyle.Render(titleStr)
	titleWidth := lipgloss.Width(titleRendered)

	actualWidth := lipgloss.Width(rendered)
	if actualWidth < titleWidth+4 {
		return rendered
	}

	topLeft := borderStyle.Render("╭─")
	topRight := borderStyle.Render(strings.Repeat("─", actualWidth-3-titleWidth) + "╮")

	lines[0] = topLeft + titleRendered + topRight

	return strings.Join(lines, "\n")
}
