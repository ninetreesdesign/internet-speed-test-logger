# Vendored browser dependencies

Files are unmodified upstream assets, except `ndt7-workers-embedded.js`, which
`build_embedded_workers.py` generates from the two worker files so the page runs from `file://`.

- M-Lab client/workers: `@m-lab/ndt7@0.1.5`, https://registry.npmjs.org/@m-lab/ndt7/-/ndt7-0.1.5.tgz
- Chart.js: `4.4.0`, https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.0/chart.umd.min.js
- Licenses: `LICENSE-ndt7` (Apache-2.0), `LICENSE-Chart.js` (MIT).

SHA-256:

- `ndt7-download-worker.js`: `2cee6e8da40e4919fa4362c6eb3ce32c21654f2c6767e45e10d0fc762511fef4`
- `ndt7-upload-worker.js`: `293e9c44eeaab299939e49bac7a3753e891054c3aed429d1ff12f529ee66942a`
- `vendor/ndt7.js`: `f01386f662ad582b6591e213c82330d142498afdb113fdbf86f30925e4ce373f`
- `vendor/chart.umd.min.js`: `e9b0f875106021fb3d58120ad8ebdd3e7d32135a4452fd8918c72ac6475f2bd3`
