local function setup(_, options)
  options = options or {}

  local default_separators = {
    angly = { "", "", "", "" },
    curvy = { "", "", "", "" },
    liney = { "", "", "|", "|" },
    empty = { "", "", "", "" }
  }
  local separators = default_separators[options.separator_style or "angly"]

  local config = {
    separator_styles = {
      separator_open = options.separator_open or separators[1],
      separator_close = options.separator_close or separators[2],
      separator_open_thin = options.separator_open_thin or separators[3],
      separator_close_thin = options.separator_close_thin or separators[4]
    },
    select_symbol = options.select_symbol or "S",
    yank_symbol = options.yank_symbol or "Y",
    filename_max_length = options.filename_max_length or 24,
    filename_trim_length = options.filename_trim_length or 6
  }

  local current_separator_style = config.separator_styles

  function Header:count()
    return ui.Line {}
  end

  function Status:mode()
    local mode = tostring(self._tab.mode):upper()
    if mode == "UNSET" then
      mode = "UN-SET"
    end

    local style = self:style()
    return ui.Span(" " .. mode .. " "):style(style)
  end

  function Status:size()
    local h = self._tab.current.hovered
    if not h then
      return ui.Line {}
    end

    local style = self:style()
    return ui.Span(current_separator_style.separator_close .. " " .. ya.readable_size(h:size() or h.cha.length) .. " ")
        :fg(style.bg):bg(THEME.status.separator_style.bg)
  end

  function Status:name()
    local h = self._tab.current.hovered
    if not h then
      return ui.Line {}
    end

    local trimmed_name = #h.name > config.filename_max_length and
        (string.sub(h.name, 1, config.filename_trim_length) .. "..." .. string.sub(h.name, -config.filename_trim_length)) or
        h.name

    local style = self:style()
    return ui.Line {
      ui.Span(current_separator_style.separator_close .. " "):fg(THEME.status.separator_style.fg),
      ui.Span(trimmed_name):fg(style.bg),
    }
  end

  function Status:files()
    local files_yanked = #cx.yanked
    local files_selected = #cx.active.selected
    local files_is_cut = cx.yanked.is_cut

    local selected_fg = files_selected > 0 and THEME.manager.count_selected.bg or THEME.status.separator_style.fg
    local yanked_fg = files_yanked > 0 and
        (files_is_cut and THEME.manager.count_cut.bg or THEME.manager.count_copied.bg) or
        THEME.status.separator_style.fg

    local yanked_text = files_yanked > 0 and config.yank_symbol .. " " .. files_yanked or config.yank_symbol .. " 0"

    return ui.Line {
      ui.Span(" " .. current_separator_style.separator_close_thin .. " "):fg(THEME.status.separator_style.fg),
      ui.Span(config.select_symbol .. " " .. files_selected .. " "):fg(selected_fg),
      ui.Span(yanked_text .. "  "):fg(yanked_fg),
    }
  end

  function Status:modified()
    local hovered = cx.active.current.hovered
    local cha = hovered.cha
    local time = (cha.modified or 0) // 1

    return ui.Span(os.date("%Y-%m-%d %H:%M", time) .. " " .. current_separator_style.separator_open_thin .. " "):fg(
      THEME.status.separator_style.fg)
  end

  function Status:percentage()
    local percent = 0
    local cursor = self._tab.current.cursor
    local length = #self._tab.current.files
    if cursor ~= 0 and length ~= 0 then
      percent = math.floor((cursor + 1) * 100 / length)
    end

    if percent == 0 then
      percent = "  Top "
    elseif percent == 100 then
      percent = "  Bot "
    else
      percent = string.format(" %3d%% ", percent)
    end

    local style = self:style()
    return ui.Line {
      ui.Span(" " .. current_separator_style.separator_open):fg(THEME.status.separator_style.fg),
      ui.Span(percent):fg(style.bg):bg(THEME.status.separator_style.bg),
      ui.Span(current_separator_style.separator_open):fg(style.bg):bg(THEME.status.separator_style.bg)
    }
  end

  function Status:position()
    local cursor = self._tab.current.cursor
    local length = #self._tab.current.files

    local style = self:style()
    return ui.Span(string.format(" %2d/%-2d ", cursor + 1, length)):style(style)
  end

  Status:children_add(Status.files, 4000, Status.LEFT)
  Status:children_add(Status.modified, 0, Status.RIGHT)
end

return { setup = setup }
