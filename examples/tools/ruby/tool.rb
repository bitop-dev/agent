#!/usr/bin/env ruby
# frozen_string_literal: true

# template — Ruby plugin tool
#
# Renders simple Mustache-style templates: {{variable}} substitution,
# {{#if condition}} blocks, {{#each list}} loops. No gems required.
#
# Protocol:
#   in:  {"type":"describe"}
#   out: {"name":"template","description":"...","parameters":{...}}
#
#   in:  {"type":"call","call_id":"c1","params":{"template":"Hello, {{name}}!","vars":{"name":"World"}}}
#   out: {"content":[{"type":"text","text":"Hello, World!"}],"error":false}
#
# Usage:
#   ruby tool.rb
#   echo '{"type":"describe"}' | ruby tool.rb

require 'json'

# ── Template engine ──────────────────────────────────────────────────────────

class TemplateEngine
  def initialize(template, vars)
    @template = template
    @vars = deep_stringify(vars)
  end

  def render
    render_block(@template, @vars)
  end

  private

  def deep_stringify(obj)
    case obj
    when Hash  then obj.transform_keys(&:to_s).transform_values { |v| deep_stringify(v) }
    when Array then obj.map { |v| deep_stringify(v) }
    else            obj
    end
  end

  # Render a template string in the context of the given vars hash.
  def render_block(tmpl, vars)
    # Process block tags first (if/each), then simple substitutions.
    result = tmpl.dup

    # {{#each list}} ... {{/each}}
    result = result.gsub(/\{\{#each\s+(\w+)\}\}(.*?)\{\{\/each\}\}/m) do
      key, body = Regexp.last_match(1), Regexp.last_match(2)
      items = vars[key]
      unless items.is_a?(Array)
        raise "each: '#{key}' is not an array (got #{items.class})"
      end
      items.map.with_index do |item, idx|
        ctx = case item
              when Hash then vars.merge(item).merge('@index' => idx, '@first' => idx == 0, '@last' => idx == items.size - 1)
              else           vars.merge('@this' => item, '@index' => idx)
              end
        render_block(body, ctx)
      end.join
    end

    # {{#if condition}} ... {{else}} ... {{/if}}
    result = result.gsub(/\{\{#if\s+(\w+)\}\}(.*?)(?:\{\{else\}\}(.*?))?\{\{\/if\}\}/m) do
      key, then_body, else_body = Regexp.last_match(1), Regexp.last_match(2), Regexp.last_match(3)
      val = vars[key]
      truthy = val && val != false && val != '' && val != 0 && !(val.is_a?(Array) && val.empty?)
      render_block(truthy ? then_body : (else_body || ''), vars)
    end

    # {{#unless condition}} ... {{/unless}}
    result = result.gsub(/\{\{#unless\s+(\w+)\}\}(.*?)\{\{\/unless\}\}/m) do
      key, body = Regexp.last_match(1), Regexp.last_match(2)
      val = vars[key]
      truthy = val && val != false && val != '' && val != 0
      render_block(truthy ? '' : body, vars)
    end

    # {{{raw}}} — unescaped (triple braces, no HTML escaping needed here)
    result = result.gsub(/\{\{\{(\w+(?:\.\w+)*)\}\}\}/) do
      lookup(vars, Regexp.last_match(1).split('.')).to_s
    end

    # {{variable}} — standard substitution with dot-path support
    result = result.gsub(/\{\{(\w+(?:\.\w+)*)\}\}/) do
      lookup(vars, Regexp.last_match(1).split('.')).to_s
    end

    result
  end

  def lookup(vars, keys)
    current = vars
    keys.each do |k|
      return '' unless current.is_a?(Hash) && current.key?(k)
      current = current[k]
    end
    current
  end
end

# ── Tool definition ──────────────────────────────────────────────────────────

DEFINITION = {
  name: 'template',
  description:
    'Render a Mustache-style template with variable substitution, ' \
    'conditionals ({{#if}}/{{else}}/{{/if}}), loops ({{#each}}...{{/each}}), ' \
    'and negation ({{#unless}}). ' \
    'Useful for generating code, config files, reports, or formatted output from structured data.',
  parameters: {
    type: 'object',
    properties: {
      template: {
        type: 'string',
        description: 'Template string with {{variable}}, {{#if key}}, {{#each list}} tags'
      },
      vars: {
        type: 'object',
        description: 'Key-value pairs substituted into the template. Values can be strings, numbers, booleans, arrays, or nested objects.'
      }
    },
    required: %w[template vars]
  }
}.freeze

# ── Protocol loop ────────────────────────────────────────────────────────────

def ok_result(text)
  { content: [{ type: 'text', text: text }], error: false }
end

def err_result(text)
  { content: [{ type: 'text', text: text }], error: true }
end

def handle_call(params)
  tmpl = params['template']
  vars = params['vars'] || {}

  return err_result("Missing required parameter: 'template'") if tmpl.nil? || !tmpl.is_a?(String)
  return err_result("Parameter 'vars' must be an object") unless vars.is_a?(Hash)

  begin
    rendered = TemplateEngine.new(tmpl, vars).render
    ok_result(rendered)
  rescue => e
    err_result("Template error: #{e.message}")
  end
end

STDOUT.sync = true

STDIN.each_line do |line|
  line = line.strip
  next if line.empty?

  begin
    msg = JSON.parse(line)
  rescue JSON::ParserError => e
    puts JSON.generate(err_result("JSON parse error: #{e.message}"))
    next
  end

  response = case msg['type']
             when 'describe'
               DEFINITION
             when 'call'
               handle_call(msg['params'] || {})
             else
               err_result("Unknown message type: #{msg['type'].inspect}")
             end

  puts JSON.generate(response)
end
