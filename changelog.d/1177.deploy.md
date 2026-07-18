Prod tripbot now runs the actual 4.4.0 image — the pin had carried a stale digest from the 4.0.0 build, so a digest override made every prod tripbot component run 4.0.0 content under the 4.4.0 tag.
