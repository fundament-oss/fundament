When we move to edition 2024 (when it's stable in aprox 3-5 months?);
- Remove `option features.(pb.go).api_level = API_OPAQUE;` since that's the default in 2024.
- Enable new feature `option features.(pb.go).strip_enum_prefix = STRIP_ENUM_PREFIX_STRIP;` for cleaner generated enums.
