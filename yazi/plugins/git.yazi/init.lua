local function string_split(input,delimiter)

	local result = {}

	for match in (input..delimiter):gmatch("(.-)"..delimiter) do
	        table.insert(result, match)
	end
	return result
end

local function set_status_color(status)
	if status == nil then
		return "#6cc749"
	elseif status == "M" then
		return "#ec613f"
	elseif status == "A" then
		return "#ec613f"
	elseif status == "." then
		return "#ae96ee"
	elseif status == "?" then
		return "#D4BB91"
	elseif status == "R" then
		return "#ec613f"
	else
		return "#ec613f"
	end
	
end

local function fix_str_ch(str)
    local chinese_chars, num_replacements = str:gsub("\\(%d%d%d)", function (s)
        return string.char(tonumber(s, 8))
    end)
    return num_replacements > 0 and chinese_chars:sub(2,-2) or chinese_chars
end

local function make_git_table(git_status_str)
	local file_table = {}
	local git_status
	local is_dirty = false
	local filename
	local multi_path
	local is_ignore_dir = false
	local is_untracked_dir = false
	local convert_name
	local split_table = string_split(git_status_str:sub(1,-2),"\n")
	for _, value in ipairs(split_table) do
		split_value = string_split(value," ")
		if split_value[#split_value - 1] == "" then
			split_value = string_split(value,"  ")
		end

		if split_value[#split_value - 1] == "??" then 
			git_status = "?"
			is_dirty = true
		elseif split_value[#split_value - 1] == "!!" then
			git_status = "."
		elseif split_value[#split_value - 1] == "->" then
			git_status = "R"
			is_dirty = true
		else
			git_status = split_value[#split_value - 1]
			is_dirty = true
		end
		if split_value[#split_value]:sub(-2,-1) == "./" and git_status == "." then
			is_ignore_dir = true
			return file_table,is_dirty,is_ignore_dir,is_untracked_dir
		end

		if split_value[#split_value]:sub(-2,-1) == "./" and git_status == "?" then
			is_untracked_dir = true
			return file_table,is_dirty,is_ignore_dir,is_untracked_dir
		end

		multi_path = string_split(split_value[#split_value],"/")
		if (multi_path[#multi_path] == "" and #multi_path == 2) or git_status ~= "." then
			filename = multi_path[1]
		else 
			filename = split_value[#split_value]
		end
		
		convert_name = fix_str_ch(filename)
		file_table[convert_name] = git_status
	end

	return file_table,is_dirty,is_ignore_dir,is_untracked_dir
end

local save = ya.sync(function(st, cwd, git_branch,git_file_status,git_is_dirty,git_status_str,is_ignore_dir,is_untracked_dir)
	if cx.active.current.cwd == Url(cwd) then
		st.git_branch = git_branch
		st.git_file_status = git_file_status
		st.git_is_dirty = git_is_dirty and "*" or ""
		st.git_status_str = git_status_str
		st.is_ignore_dir = is_ignore_dir
		st.is_untracked_dir= is_untracked_dir
		ya.render()
	end
end)

local clear_state = ya.sync(function(st)
	st.git_branch = ""
	st.git_file_status = ""
	st.git_is_dirty = ""
	ya.render()
end)

local function update_git_status(path)
	ya.manager_emit("plugin", { "git", args = ya.quote(tostring(path))})	
end

local is_in_git_dir = ya.sync(function(st)
	return (st.git_branch ~= nil and st.git_branch ~= "") and cx.active.current.cwd or nil
end)

local flush_empty_folder_status = ya.sync(function(st)
	local cwd = cx.active.current.cwd
	local folder = cx.active.current
	if #folder.window == 0 then
		clear_state()
		ya.manager_emit("plugin", { "git", args = ya.quote(tostring(cwd))})		
	end
end)

local handle_path_change = ya.sync(function(st)
	local cwd = cx.active.current.cwd
	if st.cwd ~= cwd then
		st.cwd = cwd
		clear_state()
		ya.manager_emit("plugin", { "git", args = ya.quote(tostring(cwd))})		
	end
end)


local M = {
	setup = function(st,opts)
	
		local function linemode_git(self)
			local f = self._file
			local git_span = {}
			local git_status
			if st.git_branch ~= nil and st.git_branch ~= "" then
				local name = f.name:gsub("\r", "?", 1)
				if st.is_ignore_dir then
					git_status = "."
				elseif st.is_untracked_dir then
					git_status = "?"
				elseif st.git_file_status and st.git_file_status[name] then
					git_status = st.git_file_status[name]
				else 
					git_status = nil
				end
			
				local color = set_status_color(git_status)
				if f:is_hovered() then
					git_span = (git_status ) and ui.Span(git_status .." ") or ui.Span("✓ ")	
				else
					git_span = (git_status) and ui.Span(git_status .." "):fg(color) or ui.Span("✓ "):fg(color)	
				end
			end
			return git_span
		end
		Linemode:children_add(linemode_git,8000)

		ps.sub("cd",handle_path_change)
		ps.sub("delete",flush_empty_folder_status)
		ps.sub("trash",flush_empty_folder_status)
	end,

	entry = function(_, args)
		local output
		local git_is_dirty
		local is_ignore_dir,is_untracked_dir

		local git_branch
		local command = "git symbolic-ref HEAD 2> /dev/null" 
		local file = io.popen(command, "r")
		output = file:read("*a") 
		file:close()

		if output ~= nil and  output ~= "" then
			local split_output = string_split(output:sub(1,-2),"/")
			
			git_branch = split_output[3]
		elseif is_in_git_dir() then
			git_branch = nil
		else
			return
		end
		
		local git_status_str = ""
		local git_file_status = nil
		local command = "git status --ignored -s --ignore-submodules=dirty 2> /dev/null" 
		local file = io.popen(command, "r")
		output = file:read("*a") 
		file:close()

		if output ~= nil and  output ~= "" then
			git_status_str = output
			git_file_status,git_is_dirty,is_ignore_dir,is_untracked_dir = make_git_table(git_status_str)
		end
		save(args[1], git_branch,git_file_status,git_is_dirty,git_status_str,is_ignore_dir,is_untracked_dir)
	end,
}

function M:fetch()
	local path = is_in_git_dir()
	if path then
		update_git_status(path)	
	end
	return 3
end

return M