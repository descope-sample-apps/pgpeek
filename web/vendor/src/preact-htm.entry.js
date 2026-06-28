import { h, render, Component, createContext } from "preact";
import {
  useCallback,
  useContext,
  useDebugValue,
  useEffect,
  useErrorBoundary,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "preact/hooks";
import htm from "htm";

const html = htm.bind(h);

export {
  html,
  render,
  Component,
  createContext,
  useState,
  useReducer,
  useEffect,
  useLayoutEffect,
  useRef,
  useImperativeHandle,
  useMemo,
  useCallback,
  useContext,
  useDebugValue,
  useErrorBoundary,
};
