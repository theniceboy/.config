-- Check for windows
local is_windows = ya.target_family() == "windows"
-- Define flags and strings
local is_password, is_encrypted, is_level, cmd_password, cmd_level, default_extension = false, false, false, "", "", "zip"

-- Function to check valid filename
local function is_valid_filename(name)
    -- Trim whitespace from both ends
    name = name:match("^%s*(.-)%s*$")
    if name == "" then
        return false
    end
    if is_windows then
        -- Windows forbidden chars and reserved names
        if name:find('[<>:"/\\|%?%*]') then
            return false
        end
    else
        -- Unix forbidden chars
        if name:find("/") or name:find("%z") then
            return false
        end
    end
    return true
end

-- Function to send notifications
local function notify_error(message, urgency)
    ya.notify(
        {
            title = "Archive",
            content = message,
            level = urgency,
            timeout = 5
        }
    )
end

-- Function to check if command is available
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

-- Function to change command arrays --> string -- Use first command available or first command
local function find_command_name(cmd_list)
    for _, cmd in ipairs(cmd_list) do
        if is_command_available(cmd) then
            return cmd
        end
    end
    return cmd_list[1] -- Return first command as fallback
end

-- Function to append filename to it's parent directory url
local function combine_url(path, file)
    path, file = Url(path), Url(file)
    return tostring(path:join(file))
end

-- Function to make a table of selected or hovered files: path = filenames
local selected_or_hovered =
    ya.sync(
    function()
        local tab, paths, names, path_fnames = cx.active, {}, {}, {}
        for _, u in pairs(tab.selected) do
            paths[#paths + 1] = tostring(u.parent)
            names[#names + 1] = tostring(u.name)
        end
        if #paths == 0 and tab.current.hovered then
            paths[1] = tostring(tab.current.hovered.url.parent)
            names[1] = tostring(tab.current.hovered.name)
        end
        for idx, name in ipairs(names) do
            if not path_fnames[paths[idx]] then
                path_fnames[paths[idx]] = {}
            end
            table.insert(path_fnames[paths[idx]], name)
        end
        return path_fnames, names, tostring(tab.current.cwd)
    end
)

-- Table of archive commands
local archive_commands = {
    ["%.zip$"] = {
        {command = "zip", args = {"-r"}, level_arg = "-", level_min = 0, level_max = 9, passwordable = true},
        {
            command = {"7z", "7zz", "7za"},
            args = {"a", "-tzip"},
            level_arg = "-mx=",
            level_min = 0,
            level_max = 9,
            passwordable = true
        },
        {
            command = {"tar", "bsdtar"},
            args = {"-caf"},
            level_arg = {"--option", "compression-level="},
            level_min = 1,
            level_max = 9
        }
    },
    ["%.7z$"] = {
        {
            command = {"7z", "7zz", "7za"},
            args = {"a"},
            level_arg = "-mx=",
            level_min = 0,
            level_max = 9,
            header_arg = "-mhe=on",
            passwordable = true
        }
    },
    ["%.rar$"] = {
        {
            command = "rar",
            args = {"a"},
            level_arg = "-m",
            level_min = 0,
            level_max = 5,
            header_arg = "-hp",
            passwordable = true
        }
    },
    ["%.tar.gz$"] = {
        {command = {"tar", "bsdtar"}, args = {"rpf"}, level_arg = "-", level_min = 1, level_max = 9, compress = "gzip"},
        {
            command = {"tar", "bsdtar"},
            args = {"rpf"},
            level_arg = "-mx=",
            level_min = 1,
            level_max = 9,
            compress = "7z",
            compress_args = {"a", "-tgzip"}
        },
        {
            command = {"tar", "bsdtar"},
            args = {"-czf"},
            level_arg = {"--option", "gzip:compression-level="},
            level_min = 1,
            level_max = 9
        }
    },
    ["%.tar.xz$"] = {
        {command = {"tar", "bsdtar"}, args = {"rpf"}, level_arg = "-", level_min = 1, level_max = 9, compress = "xz"},
        {
            command = {"tar", "bsdtar"},
            args = {"rpf"},
            level_arg = "-mx=",
            level_min = 1,
            level_max = 9,
            compress = "7z",
            compress_args = {"a", "-txz"}
        },
        {
            command = {"tar", "bsdtar"},
            args = {"-cJf"},
            level_arg = {"--option", "xz:compression-level="},
            level_min = 1,
            level_max = 9
        }
    },
    ["%.tar.bz2$"] = {
        {command = {"tar", "bsdtar"}, args = {"rpf"}, level_arg = "-", level_min = 1, level_max = 9, compress = "bzip2"},
        {
            command = {"tar", "bsdtar"},
            args = {"rpf"},
            level_arg = "-mx=",
            level_min = 1,
            level_max = 9,
            compress = "7z",
            compress_args = {"a", "-tbzip2"}
        },
        {
            command = {"tar", "bsdtar"},
            args = {"-cjf"},
            level_arg = {"--option", "bzip2:compression-level="},
            level_min = 1,
            level_max = 9
        }
    },
    ["%.tar.zst$"] = {
        {
            command = {"tar", "bsdtar"},
            args = {"rpf"},
            level_arg = "-",
            level_min = 1,
            level_max = 22,
            compress = "zstd",
            compress_args = {"--ultra"}
        }
    },
    ["%.tar.lz4$"] = {
        {
            command = {"tar", "bsdtar"},
            args = {"rpf"},
            level_arg = "-",
            level_min = 1,
            level_max = 12,
            compress = "lz4"
        }
    },
    ["%.tar.lha$"] = {
        {
            command = {"tar", "bsdtar"},
            args = {"rpf"},
            level_arg = "-o",
            level_min = 5,
            level_max = 7,
            compress = "lha",
            compress_args = {"-a"}
        }
    },
    ["%.tar$"] = {
        {command = {"tar", "bsdtar"}, args = {"rpf"}}
    }
}

return {
    entry = function(_, job)
        -- Parse flags and default extension
        if job.args ~= nil then
            for _, arg in ipairs(job.args) do
                if arg:match("^%-(%w+)$") then
                    -- Handle combined flags (e.g., -phl)
                    for flag in arg:sub(2):gmatch(".") do
                        if flag == "p" then
                            is_password = true
                        elseif flag == "h" then
                            is_encrypted = true
                        elseif flag == "l" then
                            is_level = true
                        end
                    end
                elseif arg:match("^%w+$") then
                    -- Handle default extension (e.g., 7z, zip)
                    if archive_commands["%." .. arg .. "$"] then
                        default_extension = arg
                    else
                        notify_error(string.format("Unsupported extension: %s", arg), "warn")
                    end
                end
            end
        end

        -- Exit visual mode
        ya.emit("escape", {visual = true})
        -- Define file table and output_dir (pwd)
        local path_fnames, fnames, output_dir = selected_or_hovered()
        -- Get archive filename
        local output_name, event =
            ya.input(
            {
                title = "Create archive:",
                position = {"top-center", y = 3, w = 40}
            }
        )
        if event ~= 1 then
            return
        end

        -- Determine the default name for the archive
        local default_name = #fnames == 1 and fnames[1] or Url(output_dir).name
        output_name = output_name == "" and string.format("%s.%s", default_name, default_extension) or output_name

        -- Add default extension if none is specified
        if not output_name:match("%.%w+$") then
            output_name = string.format("%s.%s", output_name, default_extension)
        end

        -- Validate the final archive filename
        if not is_valid_filename(output_name) then
            notify_error("Invalid archive filename", "error")
            return
        end

        -- Match user input to archive command
        local archive_cmd,
            archive_args,
            archive_compress,
            archive_level_arg,
            archive_level_min,
            archive_level_max,
            archive_header_arg,
            archive_passwordable,
            archive_compress_args
        local matched_pattern = false
        for pattern, cmd_list in pairs(archive_commands) do
            if output_name:match(pattern) then
                matched_pattern = true -- Mark that file extension is correct
                for _, cmd in ipairs(cmd_list) do
                    -- Check if archive_cmd is available
                    local find_command = type(cmd.command) == "table" and find_command_name(cmd.command) or cmd.command
                    if is_command_available(find_command) then
                        -- Check if compress_cmd (if listed) is available
                        if cmd.compress == nil or is_command_available(cmd.compress) then
                            archive_cmd = find_command
                            archive_args = cmd.args
                            archive_compress = cmd.compress or ""
                            archive_level_arg = is_level and cmd.level_arg or ""
                            archive_level_min = cmd.level_min
                            archive_level_max = cmd.level_max
                            archive_header_arg = is_encrypted and cmd.header_arg or ""
                            archive_passwordable = cmd.passwordable or false
                            archive_compress_args = cmd.compress_args or {}
                            break
                        end
                    end
                end
                if archive_cmd then
                    break
                end
            end
        end

        -- Check if no archive command is available for the extension
        if not matched_pattern then
            notify_error("Unsupported file extension", "error")
            return
        end

        -- Check if no suitable archive program was found
        if not archive_cmd then
            notify_error("Could not find a suitable archive program for the selected file extension", "error")
            return
        end

        -- Check if archive command has multiple names
        if type(archive_cmd) == "table" then
            archive_cmd = find_command_name(archive_cmd)
        end

        -- Exit if archive command is not available
        if not is_command_available(archive_cmd) then
            notify_error(string.format("%s not available", archive_cmd), "error")
            return
        end

        -- Exit if compress command is not available
        if archive_compress ~= "" and not is_command_available(archive_compress) then
            notify_error(string.format("%s compression not available", archive_compress), "error")
            return
        end

        -- Add password arg if selected
        if archive_passwordable and is_password then
            local output_password, event =
                ya.input(
                {
                    title = "Enter password:",
                    obscure = true,
                    position = {"top-center", y = 3, w = 40}
                }
            )
            if event ~= 1 then
                return
            end
            if output_password ~= "" then
                cmd_password = "-P" .. output_password
                if archive_cmd == "rar" and is_encrypted then
                    cmd_password = archive_header_arg .. output_password -- Add archive arg for rar
                end
                table.insert(archive_args, cmd_password)
            end
        end

        -- Add header arg if selected for 7z
        if is_encrypted and archive_header_arg ~= "" and archive_cmd ~= "rar" then
            table.insert(archive_args, archive_header_arg)
        end

        -- Add level arg if selected
        if archive_level_arg ~= "" and is_level then
            local output_level, event =
                ya.input(
                {
                    title = string.format("Enter compression level (%s - %s)", archive_level_min, archive_level_max),
                    position = {"top-center", y = 3, w = 40}
                }
            )
            if event ~= 1 then
                return
            end
            -- Validate user input for compression level
            if
                output_level ~= "" and tonumber(output_level) ~= nil and tonumber(output_level) >= archive_level_min and
                    tonumber(output_level) <= archive_level_max
             then
                cmd_level =
                    type(archive_level_arg) == "table" and archive_level_arg[#archive_level_arg] .. output_level or
                    archive_level_arg .. output_level
                local target_args = archive_compress == "" and archive_args or archive_compress_args
                if type(archive_level_arg) == "table" then
                    -- Insert each element of archive_level_arg (except last) into target_args at the correct position
                    for i = 1, #archive_level_arg - 1 do
                        table.insert(target_args, i, archive_level_arg[i])
                    end
                    table.insert(target_args, #archive_level_arg, cmd_level) -- Add level at the end
                else
                    -- Insert the compression level argument at the start if not a table
                    table.insert(target_args, 1, cmd_level)
                end
            else
                notify_error("Invalid level specified. Using defaults.", "warn")
            end
        end

        -- Store the original output name for later use
        local original_name = output_name

        -- If compression is needed, adjust the output name to exclude extensions like ".tar"
        if archive_compress ~= "" then
            output_name = output_name:match("(.*%.tar)") or output_name
        end

        -- Create a temporary directory for intermediate files
        local temp_dir_name = ".tmp_compress"
        local temp_dir = combine_url(output_dir, temp_dir_name)
        local temp_dir, _ = tostring(fs.unique_name(Url(temp_dir)))

        -- Attempt to create the temporary directory
        local temp_dir_status, temp_dir_err = fs.create("dir_all", Url(temp_dir))
        if not temp_dir_status then
            -- Notify the user if the temporary directory creation fails
            notify_error(string.format("Failed to create temp directory, error code: %s", temp_dir_err), "error")
            return
        end

        -- Define the temporary output file path within the temporary directory
        local temp_output_url = combine_url(temp_dir, output_name)

        -- Add files to the output archive
        for filepath, filenames in pairs(path_fnames) do
            -- Execute the archive command for each path and its respective files
            local archive_status, archive_err =
                Command(archive_cmd):arg(archive_args):arg(temp_output_url):arg(filenames):cwd(filepath):spawn():wait()
            if not archive_status or not archive_status.success then
                -- Notify the user if the archiving process fails and clean up the temporary directory
                notify_error(string.format("Failed to create archive %s with '%s', error: %s", output_name, archive_cmd, archive_err), "error")
                local cleanup_status, cleanup_err = fs.remove("dir_all", Url(temp_dir))
                if not cleanup_status then
                    notify_error(string.format("Failed to clean up temporary directory %s, error: %s", temp_dir, cleanup_err), "error")
                end
                return
            end
        end

        -- If compression is required, execute the compression command
        if archive_compress ~= "" then
            local compress_status, compress_err =
                Command(archive_compress):arg(archive_compress_args):arg(temp_output_url):spawn():wait()
            if not compress_status or not compress_status.success then
                -- Notify the user if the compression process fails and clean up the temporary directory
                notify_error(string.format("Failed to compress archive %s with '%s', error: %s", output_name, archive_compress, compress_err), "error")
                local cleanup_status, cleanup_err = fs.remove("dir_all", Url(temp_dir))
                if not cleanup_status then
                    notify_error(string.format("Failed to clean up temporary directory %s, error: %s", temp_dir, cleanup_err), "error")
                end
                return
            end
        end

        -- Move the final file from the temporary directory to the output directory
        local final_output_url, temp_url_processed = combine_url(output_dir, original_name), combine_url(temp_dir, original_name)
        final_output_url, _ = tostring(fs.unique_name(Url(final_output_url)))
        local move_status, move_err = os.rename(temp_url_processed, final_output_url)
        if not move_status then
            -- Notify the user if the move operation fails and clean up the temporary directory
            notify_error(string.format("Failed to move %s to %s, error: %s", temp_url_processed, final_output_url, move_err), "error")
            local cleanup_status, cleanup_err = fs.remove("dir_all", Url(temp_dir))
            if not cleanup_status then
                notify_error(string.format("Failed to clean up temporary directory %s, error: %s", temp_dir, cleanup_err), "error")
            end
            return
        end

        -- Cleanup the temporary directory after successful operation
        local cleanup_status, cleanup_err = fs.remove("dir_all", Url(temp_dir))
        if not cleanup_status then
            notify_error(string.format("Failed to clean up temporary directory %s, error: %s", temp_dir, cleanup_err), "error")
        end
    end
}
