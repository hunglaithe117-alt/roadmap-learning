import { describe, it, expect } from 'vitest';
import { matchRoute, routeHref } from './router';

describe('test_router_hash_mapping', () => {
  it('test_match_empty_hash_returns_hoc', () => {
    expect(matchRoute('')).toBe('/hoc');
  });

  it('test_match_review_hash_returns_review', () => {
    expect(matchRoute('#/review')).toBe('/review');
  });

  it('test_match_review_with_query_returns_review', () => {
    expect(matchRoute('#/review?deck=3')).toBe('/review');
  });

  it('test_match_caidat_hash_returns_caidat', () => {
    expect(matchRoute('#/cai-dat')).toBe('/cai-dat');
  });

  it('test_match_unknown_hash_falls_back_hoc', () => {
    expect(matchRoute('#/nope')).toBe('/hoc');
  });

  it('test_route_href_builds_hash', () => {
    expect(routeHref('/review')).toBe('#/review');
  });

  it('test_match_zh_en_drill_routes', () => {
    expect(matchRoute('#/zh-pinyin')).toBe('/zh-pinyin');
    expect(matchRoute('#/zh-stroke')).toBe('/zh-stroke');
    expect(matchRoute('#/zh-bingo')).toBe('/zh-bingo');
    expect(matchRoute('#/en-stress')).toBe('/en-stress');
    expect(matchRoute('#/en-pvo')).toBe('/en-pvo');
    expect(matchRoute('#/en-thieu')).toBe('/en-thieu');
  });

  it('test_match_player_review_routes', () => {
    expect(matchRoute('#/player')).toBe('/player');
    expect(matchRoute('#/recorder')).toBe('/recorder');
    expect(matchRoute('#/loi-sai')).toBe('/loi-sai');
    expect(matchRoute('#/reader')).toBe('/reader');
    expect(matchRoute('#/dashboard')).toBe('/dashboard');
  });

  it('test_match_player_with_query_returns_player', () => {
    expect(matchRoute('#/player?deck=2')).toBe('/player');
  });
});
