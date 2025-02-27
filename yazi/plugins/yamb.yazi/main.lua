local path_sep = package.config:sub(1, 1)

local get_hovered_path = ya.sync(function(state)
  local h = cx.active.current.hovered
  if h then
    local path = tostring(h.url)
    if h.cha.is_dir then
      return path .. path_sep
    end
    return path
  else
    return ''
  end
end)

local get_state_attr = ya.sync(function(state, attr)
  return state[attr]
end)

local set_state_attr = ya.sync(function(state, attr, value)
  state[attr] = value
end)

local set_bookmarks = ya.sync(function(state, path, value)
  state.bookmarks[path] = value
end)

local sort_bookmarks = function(bookmarks, key1, key2, reverse)
  reverse = reverse or false
  table.sort(bookmarks, function(x, y)
    if x[key1] == nil and y[key1] == nil then
      return x[key2] < y[key2]
    elseif x[key1] == nil then
      return false
    elseif y[key1] == nil then
      return true
    else
      return x[key1] < y[key1]
    end
  end)
  if reverse then
    local n = #bookmarks
    for i = 1, math.floor(n / 2) do
      bookmarks[i], bookmarks[n - i + 1] = bookmarks[n - i + 1], bookmarks[i]
    end
  end
  return bookmarks
end

local save_to_file = function(mb_path, bookmarks)
  local file = io.open(mb_path, "w")
  if file == nil then
    return
  end
  local array = {}
  for _, item in pairs(bookmarks) do
    table.insert(array, item)
  end
  sort_bookmarks(array, "tag", "key", true)
  for _, item in ipairs(array) do
    file:write(string.format("%s\t%s\t%s\n", item.tag, item.path, item.key))
  end
  file:close()
end

local fzf_find = function(cli, mb_path)
  local permit = ya.hide()
  local cmd = string.format("%s < \"%s\"", cli, mb_path)
  local handle = io.popen(cmd, "r")
  local result = ""
  if handle then
    -- strip
    result = string.gsub(handle:read("*all") or "", "^%s*(.-)%s*$", "%1")
    handle:close()
  end
  permit:drop()
  local tag, path, key = string.match(result or "", "(.-)\t(.-)\t(.*)")
  return path
end

local which_find = function(bookmarks)
  local cands = {}
  for path, item in pairs(bookmarks) do
    if #item.tag ~= 0 then
      table.insert(cands, { desc = item.tag, on = item.key, path = item.path })
    end
  end
  sort_bookmarks(cands, "on", "desc", false)
  if #cands == 0 then
    ya.notify {
      title = "Bookmarks",
      content = "Empty bookmarks",
      timeout = 2,
      level = "info",
    }
    return nil
  end
  local idx = ya.which { cands = cands }
  if idx == nil then
    return nil
  end
  return cands[idx].path
end

local action_jump = function(bookmarks, path, jump_notify)
  if path == nil then
    return
  end
  local tag = bookmarks[path].tag
  if string.sub(path, -1) == path_sep then
    ya.manager_emit("cd", { path })
  else
    ya.manager_emit("reveal", { path })
  end
  if jump_notify then
    ya.notify {
      title = "Bookmarks",
      content = 'Jump to "' .. tag .. '"',
      timeout = 2,
      level = "info",
    }
  end
end

local generate_key = function(bookmarks)
  local keys = get_state_attr("keys")
  local key2rank = get_state_attr("key2rank")
  local mb = {}
  for _, item in pairs(bookmarks) do
    if #item.key == 1 then
      table.insert(mb, item.key)
    end
  end
  if #mb == 0 then
    return keys[1]
  end
  table.sort(mb, function(a, b)
    return key2rank[a] < key2rank[b]
  end)
  local idx = 1
  for _, key in ipairs(keys) do
    if idx > #mb or key2rank[key] < key2rank[mb[idx]] then
      return key
    end
    idx = idx + 1
  end
  return nil
end

local action_save = function(mb_path, bookmarks, path)
  if path == nil or #path == 0 then
    return
  end

  local path_obj = bookmarks[path]
  -- check tag
  local tag = path_obj and path_obj.tag or path:match(".*[\\/]([^\\/]+)[\\/]?$")
  while true do
    local value, event = ya.input({
      title = "Tag (alias name)",
      value = tag,
      position = { "top-center", y = 3, w = 40 },
    })
    if event ~= 1 then
      return
    end
    tag = value or ''
    if #tag == 0 then
      ya.notify {
        title = "Bookmarks",
        content = "Empty tag",
        timeout = 2,
        level = "info",
      }
    else
      -- check the tag
      local tag_obj = nil
      for _, item in pairs(bookmarks) do
        if item.tag == tag then
          tag_obj = item
          break
        end
      end
      if tag_obj == nil or tag_obj.path == path then
        break
      end
      ya.notify {
        title = "Bookmarks",
        content = "Duplicated tag",
        timeout = 2,
        level = "info",
      }
    end
  end
  -- check key
  local key = path_obj and path_obj.key or generate_key(bookmarks)
  while true do
    local value, event = ya.input({
      title = "Key (1 character, optional)",
      value = key,
      position = { "top-center", y = 3, w = 40 },
    })
    if event ~= 1 then
      return
    end
    key = value or ""
    if key == "" then
      key = ""
      break
    elseif #key == 1 then
      -- check the key
      local key_obj = nil
      for _, item in pairs(bookmarks) do
        if item.key == key then
          key_obj = item
          break
        end
      end
      if key_obj == nil or key_obj.path == path then
        break
      else
        ya.notify {
          title = "Bookmarks",
          content = "Duplicated key",
          timeout = 2,
          level = "info",
        }
      end
    else
      ya.notify {
        title = "Bookmarks",
        content = "The length of key shoule be 1",
        timeout = 2,
        level = "info",
      }
    end
  end
  -- save
  set_bookmarks(path, { tag = tag, path = path, key = key })
  bookmarks = get_state_attr("bookmarks")
  save_to_file(mb_path, bookmarks)
  ya.notify {
    title = "Bookmarks",
    content = '"' .. tag .. '" saved"',
    timeout = 2,
    level = "info",
  }
end

local action_delete = function(mb_path, bookmarks, path)
  if path == nil then
    return
  end
  local tag = bookmarks[path].tag
  set_bookmarks(path, nil)
  bookmarks = get_state_attr("bookmarks")
  save_to_file(mb_path, bookmarks)
  ya.notify {
    title = "Bookmarks",
    content = '"' .. tag .. '" deleted',
    timeout = 2,
    level = "info",
  }
end

local action_delete_all = function(mb_path)
  local value, event = ya.input({
    title = "Delete all bookmarks? (y/n)",
    position = { "top-center", y = 3, w = 40 },
  })
  if event ~= 1 then
    return
  end
  if string.lower(value) == "y" then
    set_state_attr("bookmarks", {})
    save_to_file(mb_path, {})
    ya.notify {
      title = "Bookmarks",
      content = "All bookmarks deleted",
      timeout = 2,
      level = "info",
    }
  else
    ya.notify {
      title = "Bookmarks",
      content = "Cancel delete",
      timeout = 2,
      level = "info",
    }
  end
end

return {
  setup = function(state, options)
    state.path = options.path or
        (ya.target_family() == "windows" and os.getenv("APPDATA") .. "\\yazi\\config\\bookmark") or
        (os.getenv("HOME") .. "/.config/yazi/bookmark")
    state.cli = options.cli or "fzf"
    state.jump_notify = options.jump_notify and true
    -- init the keys
    local keys = options.keys or "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
    state.keys = {}
    state.key2rank = {}
    for i = 1, #keys do
      local char = keys:sub(i, i)
      table.insert(state.keys, char)
      state.key2rank[char] = i
    end

    -- init the bookmarks
    local bookmarks = {}
    for _, item in pairs(options.bookmarks or {}) do
      bookmarks[item.path] = { tag = item.tag, path = item.path, key = item.key }
    end
    -- load the config
    local file = io.open(state.path, "r")
    if file ~= nil then
      for line in file:lines() do
        local tag, path, key = string.match(line, "(.-)\t(.-)\t(.*)")
        if tag and path then
          key = key or ""
          bookmarks[path] = { tag = tag, path = path, key = key }
        end
      end
      file:close()
    end
    -- create bookmarks file to enable fzf
    save_to_file(state.path, bookmarks)
    state.bookmarks = bookmarks
  end,
  entry = function(self, jobs)
    local action = jobs.args[1]
    if not action then
      return
    end
    local mb_path, cli, bookmarks, jump_notify = get_state_attr("path"), get_state_attr("cli"), get_state_attr("bookmarks"), get_state_attr("jump_notify")
    if action == "save" then
      action_save(mb_path, bookmarks, get_hovered_path())
    elseif action == "delete_by_key" then
      action_delete(mb_path, bookmarks, which_find(bookmarks))
    elseif action == "delete_by_fzf" then
      action_delete(mb_path, bookmarks, fzf_find(cli, mb_path))
    elseif action == "delete_all" then
      action_delete_all(mb_path)
    elseif action == "jump_by_key" then
      action_jump(bookmarks, which_find(bookmarks), jump_notify)
    elseif action == "jump_by_fzf" then
      action_jump(bookmarks, fzf_find(cli, mb_path), jump_notify)
    elseif action == "rename_by_key" then
      action_save(mb_path, bookmarks, which_find(bookmarks))
    elseif action == "rename_by_fzf" then
      action_save(mb_path, bookmarks, fzf_find(cli, mb_path))
    end
  end,
}
