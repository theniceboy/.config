---@diagnostic disable: undefined-global

local function setup(_, options)
	options = options or {}

	local default_separators = {
		angly = { "", "", "", "" },
		curvy = { "", "", "", "" },
		liney = { "", "", "|", "|" },
		empty = { "", "", "", "" },
	}
	local separators = default_separators[options.separator_style or "angly"]

	local config = {
		separator_styles = {
			separator_open = options.separator_open or separators[1],
			separator_close = options.separator_close or separators[2],
			separator_open_thin = options.separator_open_thin or separators[3],
			separator_close_thin = options.separator_close_thin or separators[4],
			separator_head = options.separator_head or "",
			separator_tail = options.separator_tail or "",
		},
		select_symbol = options.select_symbol or "S",
		yank_symbol = options.yank_symbol or "Y",

		filename_max_length = options.filename_max_length or 24,
		filename_truncate_length = options.filename_truncate_length or 6,
		filename_truncate_separator = options.filename_truncate_separator or "...",

		color = options.color or nil,
    secondary_color = options.secondary_color or nil,
    default_files_color = options.default_files_color
      or th.which.separator_style.fg
      or "darkgray",
    selected_files_color = options.selected_files_color
      or th.mgr.count_selected.bg
      or "white",
    yanked_files_color = options.selected_files_color
      or th.mgr.count_copied.bg
      or "green",
    cut_files_color = options.cut_files_color
      or th.mgr.count_cut.bg
      or "red",
	}

	local current_separator_style = config.separator_styles

	function Header:count()
		return ui.Line({})
	end

	function Status:mode()
		local mode = tostring(self._tab.mode):upper()

		local style = self:style()
		return ui.Line({
			ui.Span(current_separator_style.separator_head)
        :fg(config.color or style.main.bg),
			ui.Span(" " .. mode .. " ")
        :fg(th.which.mask.bg)
        :bg(config.color or style.main.bg),
		})
	end

	function Status:size()
		local h = self._current.hovered
		local size = h and (h:size() or h.cha.len) or 0

		local style = self:style()
		return ui.Span(current_separator_style.separator_close .. " " .. ya.readable_size(size) .. " ")
			:fg(config.color or style.main.bg)
			:bg(config.secondary_color or th.which.separator_style.fg)
	end

	function Status:utf8_sub(str, start_char, end_char)
		local start_byte = utf8.offset(str, start_char)
		local end_byte = end_char and (utf8.offset(str, end_char + 1) - 1) or #str

		if not start_byte or not end_byte then
			return ""
		end

		return string.sub(str, start_byte, end_byte)
	end

	function Status:truncate_name(filename, max_length)
		local base_name, extension = filename:match("^(.+)(%.[^%.]+)$")
		base_name = base_name or filename
		extension = extension or ""

		if utf8.len(base_name) > max_length then
			base_name = self:utf8_sub(base_name, 1, config.filename_truncate_length)
				.. config.filename_truncate_separator
				.. self:utf8_sub(base_name, -config.filename_truncate_length)
		end

		return base_name .. extension
	end

	function Status:name()
		local h = self._current.hovered
		if not h then
			return ui.Line({
				ui.Span(current_separator_style.separator_close .. " ")
          :fg(config.secondary_color or th.which.separator_style.fg),
				ui.Span("Empty dir")
          :fg(config.color or style.main.bg),
			})
		end

		local truncated_name = self:truncate_name(h.name, config.filename_max_length)

		local style = self:style()
		return ui.Line({
			ui.Span(current_separator_style.separator_close .. " ")
        :fg(config.secondary_color or th.which.separator_style.fg),
			ui.Span(truncated_name)
        :fg(config.color or style.main.bg),
		})
	end

	function Status:files()
		local files_yanked = #cx.yanked
		local files_selected = #cx.active.selected
		local files_cut = cx.yanked.is_cut

		local selected_fg = files_selected > 0
      and config.selected_files_color
      or config.default_files_color
		local yanked_fg = files_yanked > 0
      and
      (files_cut
        and config.cut_files_color
        or config.yanked_files_color
      )
			or config.default_files_color

		local yanked_text = files_yanked > 0
      and config.yank_symbol .. " " .. files_yanked
      or config.yank_symbol .. " 0"

		return ui.Line({
			ui.Span(" " .. current_separator_style.separator_close_thin .. " ")
        :fg(th.which.separator_style.fg),
			ui.Span(config.select_symbol .. " " .. files_selected .. " ")
        :fg(selected_fg),
			ui.Span(yanked_text .. "  ")
        :fg(yanked_fg),
		})
	end

	function Status:modified()
		local hovered = cx.active.current.hovered

		if not hovered then
			return ""
		end

		local cha = hovered.cha
		local time = (cha.mtime or 0) // 1

		return ui.Span(os.date("%Y-%m-%d %H:%M", time) .. " " .. current_separator_style.separator_open_thin .. " ")
			:fg(th.which.separator_style.fg)
	end

	function Status:percent()
		local percent = 0
		local cursor = self._tab.current.cursor
		local length = #self._tab.current.files
		if cursor ~= 0 and length ~= 0 then
			percent = math.floor((cursor + 1) * 100 / length)
		end

		if percent == 0 then
			percent = " Top "
		elseif percent == 100 then
			percent = " Bot "
		else
			percent = string.format(" %2d%% ", percent)
		end

		local style = self:style()
		return ui.Line({
      ui.Span(" " .. current_separator_style.separator_open)
        :fg(config.secondary_color or th.which.separator_style.fg),
			ui.Span(percent)
        :fg(config.color or style.main.bg)
        :bg(config.secondary_color or th.which.separator_style.fg),
			ui.Span(current_separator_style.separator_open)
				:fg(config.color or style.main.bg)
				:bg(config.secondary_color or th.which.separator_style.fg),
		})
	end

	function Status:position()
		local cursor = self._tab.current.cursor
		local length = #self._tab.current.files

		local style = self:style()
		return ui.Line({
			ui.Span(string.format(" %2d/%-2d ", math.min(cursor + 1, length), length))
				:fg(th.which.mask.bg)
				:bg(config.color or style.main.bg),
			ui.Span(current_separator_style.separator_tail):fg(config.color or style.main.bg),
		})
	end

	Status:children_add(Status.files, 4000, Status.LEFT)
	Status:children_add(Status.modified, 0, Status.RIGHT)
end

return { setup = setup }