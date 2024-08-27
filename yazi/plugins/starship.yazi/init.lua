local save = ya.sync(function(st, cwd, output)
    if cx.active.current.cwd == Url(cwd) then
        st.output = output
        ya.render()
    end
end)

-- Helper function for accessing the `config_file` state variable
---@return string
local get_config_file = ya.sync(function(st)
    return st.config_file
end)

return {
    ---User arguments for setup method
    ---@class SetupArgs
    ---@field config_file string Absolute path to a starship config file

    --- Setup plugin
    --- @param st table State
    --- @param args SetupArgs|nil
    setup = function(st, args)
        -- Replace default header widget
        Header:children_remove(1, Header.LEFT)
        Header:children_add(function()
            return ui.Line.parse(st.output or "")
        end, 1000, Header.LEFT)

        -- Check for custom starship config file
        if args ~= nil and args.config_file ~= nil then
            local url = Url(args.config_file)
            if url.is_regular then
                local config_file = args.config_file

                -- Manually replace '~' and '$HOME' at the start of the path with the OS environment variable
                local home = os.getenv("HOME")
                if home then
                    home = tostring(home)
                    config_file = config_file:gsub("^~", home):gsub("^$HOME", home)
                end

                st.config_file = config_file
            end
        end

        -- Pass current working directory and custom config path (if specified) to the plugin's entry point
        ---Callback for subscribers to update the prompt
        local callback = function()
            local cwd = cx.active.current.cwd
            if st.cwd ~= cwd then
                st.cwd = cwd
                ya.manager_emit("plugin", {
                    st._id,
                    args = ya.quote(tostring(cwd), true),
                })
            end
        end

        -- Subscribe to events
        ps.sub("cd", callback)
        ps.sub("tab", callback)
    end,

    entry = function(_, args)
        local command = Command("starship"):arg("prompt"):cwd(args[1]):env("STARSHIP_SHELL", "")

        -- Point to custom starship config
        local config_file = get_config_file()
        if config_file then
            command = command:env("STARSHIP_CONFIG", config_file)
        end

        local output = command:output()
        if output then
            save(args[1], output.stdout:gsub("^%s+", ""))
        end
    end,
}
