-- Send error notification
local function notify_error(message, urgency)
	ya.notify({
		title = "Archive",
		content = message,
		level = urgency,
		timeout = 5,
	})
end

-- Check for windows
local is_windows = ya.target_family() == "windows"

-- Make table of selected or hovered: path = filenames
local selected_or_hovered = ya.sync(function()
	local tab, paths, names, path_fnames = cx.active, {}, {}, {}
	for _, u in pairs(tab.selected) do
		paths[#paths + 1] = tostring(u:parent())
		names[#names + 1] = tostring(u:name())
	end
	if #paths == 0 and tab.current.hovered then
		paths[1] = tostring(tab.current.hovered.url:parent())
		names[1] = tostring(tab.current.hovered.name)
	end
	for idx, name in ipairs(names) do
		if not path_fnames[paths[idx]] then
			path_fnames[paths[idx]] = {}
		end
		table.insert(path_fnames[paths[idx]], name)
	end
	return path_fnames, tostring(tab.current.cwd)
end)

-- Check if archive command is available
local function is_command_available(cmd)
	local stat_cmd

	if is_windows then
		stat_cmd = string.format("where %s > nul 2>&1", cmd)
	else
		stat_cmd = string.format("command -v %s >/dev/null 2>&1", cmd)
	end

	local cmd_exists = os.execute(stat_cmd)
	if cmd_exists then
		return true
	else
		return false
	end
end

-- Archive command list --> string
local function find_binary(cmd_list)
	for _, cmd in ipairs(cmd_list) do
		if is_command_available(cmd) then
			return cmd
		end
	end
	return cmd_list[1] -- Return first command as fallback
end

-- Check if file exists
local function file_exists(name)
	local f = io.open(name, "r")
	if f ~= nil then
		io.close(f)
		return true
	else
		return false
	end
end

-- Append filename to it's parent directory
local function combine_url(path, file)
	path, file = Url(path), Url(file)
	return tostring(path:join(file))
end

return {
	entry = function()
		-- Exit visual mode
		ya.manager_emit("escape", { visual = true })

		-- Define file table and output_dir (pwd)
		local path_fnames, output_dir = selected_or_hovered()

		-- Get input
		local output_name, event = ya.input({
			title = "Create archive:",
			position = { "top-center", y = 3, w = 40 },
		})
		if event ~= 1 then
			return
		end

		-- Use appropriate archive command
		local archive_commands = {
			["%.zip$"] = { command = "zip", args = { "-r" } },
			["%.7z$"] = { command = { "7z", "7zz" }, args = { "a" } },
			["%.tar.gz$"] = { command = "tar", args = { "rpf" }, compress = "gzip" },
			["%.tar.xz$"] = { command = "tar", args = { "rpf" }, compress = "xz" },
			["%.tar.bz2$"] = { command = "tar", args = { "rpf" }, compress = "bzip2" },
			["%.tar.zst$"] = { command = "tar", args = { "rpf" }, compress = "zstd", compress_args = { "--rm" } },
			["%.tar$"] = { command = "tar", args = { "rpf" } },
		}

		if is_windows then
			archive_commands = {
				["%.zip$"] = { command = "7z", args = { "a", "-tzip" } },
				["%.7z$"] = { command = "7z", args = { "a" } },
				["%.tar.gz$"] = {
					command = "tar",
					args = { "rpf" },
					compress = "7z",
					compress_args = { "a", "-tgzip", "-sdel", output_name },
				},
				["%.tar.xz$"] = {
					command = "tar",
					args = { "rpf" },
					compress = "7z",
					compress_args = { "a", "-txz", "-sdel", output_name },
				},
				["%.tar.bz2$"] = {
					command = "tar",
					args = { "rpf" },
					compress = "7z",
					compress_args = { "a", "-tbzip2", "-sdel", output_name },
				},
				["%.tar.zst$"] = { command = "tar", args = { "rpf" }, compress = "zstd", compress_args = { "--rm" } },
				["%.tar$"] = { command = "tar", args = { "rpf" } },
			}
		end

		-- Match user input to archive command
		local archive_cmd, archive_args, archive_compress, archive_compress_args
		for pattern, cmd_pair in pairs(archive_commands) do
			if output_name:match(pattern) then
				archive_cmd = cmd_pair.command
				archive_args = cmd_pair.args
				archive_compress = cmd_pair.compress
				archive_compress_args = cmd_pair.compress_args or {}
			end
		end

		-- Check if archive command has multiple names
		if type(archive_cmd) == "table" then
			archive_cmd = find_binary(archive_cmd)
		end

		-- Check if no archive command is available for the extention
		if not archive_cmd then
			notify_error("Unsupported file extention", "error")
			return
		end

		-- Exit if archive command is not available
		if not is_command_available(archive_cmd) then
			notify_error(string.format("%s not available", archive_cmd), "error")
			return
		end

		-- Exit if compress command is not available
		if archive_compress and not is_command_available(archive_compress) then
			notify_error(string.format("%s compression not available", archive_compress), "error")
			return
		end

		-- If file exists show overwrite prompt
		local output_url = combine_url(output_dir, output_name)
		while true do
			if file_exists(output_url) then
				local overwrite_answer = ya.input({
					title = "Overwrite " .. output_name .. "? y/N:",
					position = { "top-center", y = 3, w = 40 },
				})
				if overwrite_answer:lower() ~= "y" then
					notify_error("Operation canceled", "warn")
					return -- If no overwrite selected, exit
				else
					local rm_status, rm_err = os.remove(output_url)
					if not rm_status then
						notify_error(string.format("Failed to remove %s, exit code %s", output_name, rm_err), "error")
						return
					end -- If overwrite fails, exit
				end
			end
			if archive_compress and not output_name:match("%.tar$") then
				output_name = output_name:match("(.*%.tar)") -- Test for .tar and .tar.*
				output_url = combine_url(output_dir, output_name) -- Update output_url
			else
				break
			end
		end

		-- Add to output archive in each path, their respective files
		for path, names in pairs(path_fnames) do
			local archive_status, archive_err =
				Command(archive_cmd):args(archive_args):arg(output_url):args(names):cwd(path):spawn():wait()
			if not archive_status or not archive_status.success then
				notify_error(
					string.format(
						"%s with selected files failed, exit code %s",
						archive_args,
						archive_status and archive_status.code or archive_err
					),
					"error"
				)
			end
		end

		-- Use compress command if needed
		if archive_compress then
			local compress_status, compress_err =
				Command(archive_compress):args(archive_compress_args):arg(output_name):cwd(output_dir):spawn():wait()
			if not compress_status or not compress_status.success then
				notify_error(
					string.format(
						"%s with %s failed, exit code %s",
						archive_compress,
						output_name,
						compress_status and compress_status.code or compress_err
					),
					"error"
				)
			end
		end
	end,
}
